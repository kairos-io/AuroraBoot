package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/auth"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

var _ = Describe("CommandHandler", func() {
	var (
		e       *echo.Echo
		ns      *fakeNodeStore
		cs      *fakeCommandStore
		handler *handlers.CommandHandler
	)

	BeforeEach(func() {
		e = echo.New()
		ns = &fakeNodeStore{
			nodes: []*store.ManagedNode{
				{ID: "node-1", GroupID: "grp-1"},
				{ID: "node-2", GroupID: "grp-1"},
			},
		}
		cs = &fakeCommandStore{}
		handler = handlers.NewCommandHandler(cs, ns, nil)
	})

	Describe("Create", func() {
		It("should create a command for a node", func() {
			body := `{"command":"upgrade","args":{"version":"1.2.0"}}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")

			err := handler.Create(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusCreated))

			var cmd store.NodeCommand
			Expect(json.Unmarshal(rec.Body.Bytes(), &cmd)).To(Succeed())
			Expect(cmd.Command).To(Equal("upgrade"))
			Expect(cmd.ManagedNodeID).To(Equal("node-1"))
			Expect(cmd.Phase).To(Equal(store.CommandPending))
		})

		It("should reject command without command field", func() {
			body := `{"args":{"version":"1.2.0"}}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")

			err := handler.Create(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("CreateBulk", func() {
		It("should create commands for selected nodes", func() {
			body := `{"selector":{"nodeIDs":["node-1","node-2"]},"command":"upgrade","args":{"version":"2.0"}}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.CreateBulk(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusCreated))

			var cmds []*store.NodeCommand
			Expect(json.Unmarshal(rec.Body.Bytes(), &cmds)).To(Succeed())
			Expect(cmds).To(HaveLen(2))
		})
	})

	Describe("CreateForGroup", func() {
		It("should create commands for all nodes in a group", func() {
			body := `{"command":"reset"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/grp-1/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("grp-1")

			err := handler.CreateForGroup(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusCreated))

			var cmds []*store.NodeCommand
			Expect(json.Unmarshal(rec.Body.Bytes(), &cmds)).To(Succeed())
			Expect(cmds).To(HaveLen(2))
		})
	})

	Describe("DELETE /nodes/:nodeID/commands/:commandID", func() {
		It("should delete a completed command and return 204", func() {
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-del-1", ManagedNodeID: "node-1", Phase: store.CommandCompleted, Result: "done"},
			}
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/nodes/node-1/commands/cmd-del-1", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID", "commandID")
			c.SetParamValues("node-1", "cmd-del-1")

			err := handler.Delete(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusNoContent))

			// Verify command is gone from the fake store.
			_, lookupErr := cs.GetByID(nil, "cmd-del-1")
			Expect(lookupErr).To(HaveOccurred())
		})

		It("should return 404 for non-existent command", func() {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/nodes/node-1/commands/nonexistent", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID", "commandID")
			c.SetParamValues("node-1", "nonexistent")

			err := handler.Delete(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusNotFound))
		})
	})

	Describe("DELETE /nodes/:nodeID/commands (ClearHistory)", func() {
		It("should delete all terminal commands for the node", func() {
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-ch-1", ManagedNodeID: "node-1", Phase: store.CommandCompleted},
				{ID: "cmd-ch-2", ManagedNodeID: "node-1", Phase: store.CommandFailed},
				{ID: "cmd-ch-3", ManagedNodeID: "node-1", Phase: store.CommandRunning},
			}
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/nodes/node-1/commands", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")

			err := handler.ClearHistory(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusNoContent))

			// Only the Running command should remain.
			remaining, _ := cs.ListByNode(nil, "node-1")
			Expect(remaining).To(HaveLen(1))
			Expect(remaining[0].Phase).To(Equal(store.CommandRunning))
		})

		It("should return 204 even if no commands to delete", func() {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/nodes/node-1/commands", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")

			err := handler.ClearHistory(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusNoContent))
		})
	})

	Describe("UpdateStatus", func() {
		It("should update a command's status", func() {
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-1", ManagedNodeID: "node-1", Phase: store.CommandDelivered},
			}
			body := `{"phase":"Completed","result":"success"}`
			req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/node-1/commands/cmd-1/status", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID", "commandID")
			c.SetParamValues("node-1", "cmd-1")

			err := handler.UpdateStatus(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))
		})

		It("should reject update without phase", func() {
			body := `{"result":"success"}`
			req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/node-1/commands/cmd-1/status", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID", "commandID")
			c.SetParamValues("node-1", "cmd-1")

			err := handler.UpdateStatus(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})

		// Agent callers (node API key auth sets the node identity in context)
		// may only update commands addressed to them. The path :nodeID has
		// already been bound to the identity by RequireNodeMatch, so the risk
		// here is a foreign commandID under the node's own path.
		It("lets an agent update its own command", func() {
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-own", ManagedNodeID: "node-1", Phase: store.CommandDelivered},
			}
			body := `{"phase":"Completed","result":"ok"}`
			req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/node-1/commands/cmd-own/status", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID", "commandID")
			c.SetParamValues("node-1", "cmd-own")
			c.Set(auth.ContextKeyNodeID, "node-1")

			Expect(handler.UpdateStatus(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(cs.cmds[0].Phase).To(Equal(store.CommandCompleted))
		})

		It("forbids an agent updating a command it does not own (403, unchanged)", func() {
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-foreign", ManagedNodeID: "node-2", Phase: store.CommandDelivered},
			}
			body := `{"phase":"Completed","result":"pwned"}`
			req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/node-1/commands/cmd-foreign/status", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID", "commandID")
			c.SetParamValues("node-1", "cmd-foreign")
			c.Set(auth.ContextKeyNodeID, "node-1")

			Expect(handler.UpdateStatus(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusForbidden))
			// Foreign command must be untouched.
			Expect(cs.cmds[0].Phase).To(Equal(store.CommandDelivered))
		})

		It("forbids an agent updating a non-existent command (403, not silent 200)", func() {
			body := `{"phase":"Completed","result":"x"}`
			req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/node-1/commands/missing/status", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID", "commandID")
			c.SetParamValues("node-1", "missing")
			c.Set(auth.ContextKeyNodeID, "node-1")

			Expect(handler.UpdateStatus(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusForbidden))
		})
	})
})
