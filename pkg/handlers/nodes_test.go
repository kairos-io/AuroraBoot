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
	"github.com/kairos-io/AuroraBoot/pkg/ws"
	"github.com/labstack/echo/v4"
)

var _ = Describe("NodeHandler", func() {
	var (
		e       *echo.Echo
		ns      *fakeNodeStore
		cs      *fakeCommandStore
		gs      *fakeGroupStore
		hub     *ws.Hub
		handler *handlers.NodeHandler
	)

	BeforeEach(func() {
		e = echo.New()
		ns = &fakeNodeStore{}
		cs = &fakeCommandStore{}
		gs = &fakeGroupStore{}
		hub = ws.NewHub()
		handler = handlers.NewNodeHandler(ns, cs, gs, hub, "reg-token", "http://localhost:8080")
	})

	Describe("Register", func() {
		It("should register a new node", func() {
			body := `{"registrationToken":"reg-token","machineID":"machine-1","hostname":"host-1","agentVersion":"1.0"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Register(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusCreated))

			var resp map[string]interface{}
			Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["id"]).NotTo(BeEmpty())
			Expect(resp["apiKey"]).NotTo(BeEmpty())
		})

		It("should return existing node if machineID already registered", func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "existing-id", MachineID: "machine-1", APIKey: "existing-key"},
			}
			body := `{"registrationToken":"reg-token","machineID":"machine-1","hostname":"host-1"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Register(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var resp map[string]interface{}
			Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["id"]).To(Equal("existing-id"))
			Expect(resp["apiKey"]).To(Equal("existing-key"))
		})

		It("should reject registration without machineID", func() {
			body := `{"registrationToken":"reg-token","hostname":"host-1"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Register(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("List", func() {
		BeforeEach(func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-1", GroupID: "grp-1", Labels: map[string]string{"env": "prod"}},
				{ID: "node-2", GroupID: "grp-2", Labels: map[string]string{"env": "staging"}},
			}
		})

		It("should list all nodes", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.List(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var nodes []*store.ManagedNode
			Expect(json.Unmarshal(rec.Body.Bytes(), &nodes)).To(Succeed())
			Expect(nodes).To(HaveLen(2))
		})

		It("should filter by group", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?group=grp-1", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.List(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var nodes []*store.ManagedNode
			Expect(json.Unmarshal(rec.Body.Bytes(), &nodes)).To(Succeed())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].ID).To(Equal("node-1"))
		})

		It("should filter by label", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?label=env:staging", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.List(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var nodes []*store.ManagedNode
			Expect(json.Unmarshal(rec.Body.Bytes(), &nodes)).To(Succeed())
			Expect(nodes).To(HaveLen(1))
			Expect(nodes[0].ID).To(Equal("node-2"))
		})
	})

	Describe("Get", func() {
		It("should return a node by ID", func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-1", Hostname: "host-1"},
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")

			err := handler.Get(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))
		})

		It("should return 404 for missing node", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/missing", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("missing")

			err := handler.Get(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusNotFound))
		})
	})

	Describe("Delete", func() {
		It("should delete a node", func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-1"},
			}
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/nodes/node-1", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")

			err := handler.Delete(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusNoContent))
		})
	})

	// Decommission is the remote-teardown entry point: it creates an
	// `unregister` NodeCommand and pushes it down the node's live WS
	// connection (if any). It does NOT delete the node record — the UI
	// drives DELETE as a second step once the command finishes.
	Describe("Decommission", func() {
		decommission := func(nodeID string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/"+nodeID+"/decommission", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues(nodeID)
			Expect(handler.Decommission(c)).To(Succeed())
			return rec
		}

		It("returns 404 when the node doesn't exist", func() {
			rec := decommission("missing")
			Expect(rec.Code).To(Equal(http.StatusNotFound))
		})

		It("reports nodeOnline=false and queues no command when the node is offline", func() {
			ns.nodes = []*store.ManagedNode{{ID: "node-offline"}}
			// hub has no registered connection for node-offline, so IsOnline is false

			rec := decommission("node-offline")
			Expect(rec.Code).To(Equal(http.StatusOK))

			var resp map[string]interface{}
			Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["nodeOnline"]).To(Equal(false))
			Expect(resp["commandID"]).To(Equal(""))
			// Nothing is persisted for the offline path — the operator will
			// force-delete and run the CLI fallback on the box.
			Expect(cs.cmds).To(BeEmpty())
		})

		// Online-path coverage lives in the integration test (needs a real
		// *websocket.Conn inside the hub to satisfy IsOnline). Hub exposes
		// no test hook for synthesizing an online node in unit tests, so
		// asserting the persisted command's shape there would require
		// reaching into unexported hub state — deliberately skipped here.
	})

	Describe("SetLabels", func() {
		It("should set labels on a node", func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-1", Labels: map[string]string{}},
			}
			body := `{"labels":{"env":"production","tier":"web"}}`
			req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/node-1/labels", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")

			err := handler.SetLabels(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("SetGroup", func() {
		It("should set a group on a node", func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-1"},
			}
			body := `{"groupID":"grp-1"}`
			req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/node-1/group", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")

			err := handler.SetGroup(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})

	Describe("Heartbeat", func() {
		It("should update heartbeat and phase", func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-1", Phase: store.PhaseRegistered},
			}
			body := `{"agentVersion":"1.1","osRelease":{"ID":"ubuntu"}}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/heartbeat", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")

			err := handler.Heartbeat(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(ns.nodes[0].Phase).To(Equal(store.PhaseOnline))
		})
	})

	Describe("GetCommands", func() {
		It("should return pending commands for agent and mark delivered", func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-1"},
			}
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-1", ManagedNodeID: "node-1", Command: "upgrade", Phase: store.CommandPending},
				{ID: "cmd-2", ManagedNodeID: "node-1", Command: "exec", Phase: store.CommandCompleted},
			}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/commands", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")
			// Simulate agent auth by setting nodeID in context
			c.Set(auth.ContextKeyNodeID, "node-1")

			err := handler.GetCommands(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var cmds []*store.NodeCommand
			Expect(json.Unmarshal(rec.Body.Bytes(), &cmds)).To(Succeed())
			Expect(cmds).To(HaveLen(1))
			Expect(cmds[0].ID).To(Equal("cmd-1"))
		})

		It("should return all commands for admin", func() {
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-1", ManagedNodeID: "node-1", Phase: store.CommandPending},
				{ID: "cmd-2", ManagedNodeID: "node-1", Phase: store.CommandCompleted},
			}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/commands", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")
			// No nodeID in context = admin request

			err := handler.GetCommands(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var cmds []*store.NodeCommand
			Expect(json.Unmarshal(rec.Body.Bytes(), &cmds)).To(Succeed())
			Expect(cmds).To(HaveLen(2))
		})
	})

	Describe("InstallScript", func() {
		It("should return a bash script", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/install-agent", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.InstallScript(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(ContainSubstring("#!/bin/bash"))
			Expect(rec.Body.String()).To(ContainSubstring("http://localhost:8080"))
		})
	})
})
