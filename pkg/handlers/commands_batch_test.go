package handlers_test

import (
	"context"
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

// A fan-out is one operation, not N unrelated commands (kairos-io/kairos#4267).
// These specs pin the three things that makes true: every command of a fan-out
// shares a batch id, a failure stops the rest of a fail-fast batch without
// touching what has already been handed to a node, and the batch reads back as
// a single status.
var _ = Describe("CommandHandler batches", func() {
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
				{ID: "node-3", GroupID: "grp-1"},
			},
		}
		cs = &fakeCommandStore{}
		handler = handlers.NewCommandHandler(cs, ns, nil, nil, nil)
	})

	// postBulk fires the selector endpoint and returns the commands it created.
	postBulk := func(body string) []*store.NodeCommand {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/commands", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		ExpectWithOffset(1, handler.CreateBulk(e.NewContext(req, rec))).To(Succeed())
		ExpectWithOffset(1, rec.Code).To(Equal(http.StatusCreated))
		var cmds []*store.NodeCommand
		ExpectWithOffset(1, json.Unmarshal(rec.Body.Bytes(), &cmds)).To(Succeed())
		return cmds
	}

	// reportStatus makes the node behind cmd report a phase, the way an agent
	// does over the REST status endpoint.
	reportStatus := func(nodeID, commandID, phase string) *httptest.ResponseRecorder {
		body := `{"phase":"` + phase + `","result":"reported"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/"+nodeID+"/commands/"+commandID+"/status", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("nodeID", "commandID")
		c.SetParamValues(nodeID, commandID)
		c.Set(auth.ContextKeyNodeID, nodeID)
		ExpectWithOffset(1, handler.UpdateStatus(c)).To(Succeed())
		return rec
	}

	find := func(id string) *store.NodeCommand {
		for _, cmd := range cs.cmds {
			if cmd.ID == id {
				return cmd
			}
		}
		return nil
	}

	Describe("batch identity", func() {
		It("gives every command of one fan-out the same batch id", func() {
			cmds := postBulk(`{"selector":{"nodeIDs":["node-1","node-2","node-3"]},"command":"upgrade"}`)
			Expect(cmds).To(HaveLen(3))
			Expect(cmds[0].BatchID).NotTo(BeEmpty())
			Expect(cmds[1].BatchID).To(Equal(cmds[0].BatchID))
			Expect(cmds[2].BatchID).To(Equal(cmds[0].BatchID))
		})

		It("gives two fan-outs different batch ids", func() {
			first := postBulk(`{"selector":{"nodeIDs":["node-1"]},"command":"upgrade"}`)
			second := postBulk(`{"selector":{"nodeIDs":["node-2"]},"command":"upgrade"}`)
			Expect(first[0].BatchID).NotTo(Equal(second[0].BatchID))
		})

		It("gives a group fan-out a batch id too", func() {
			body := `{"command":"upgrade","failFast":true}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/grp-1/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("grp-1")
			Expect(handler.CreateForGroup(c)).To(Succeed())

			var cmds []*store.NodeCommand
			Expect(json.Unmarshal(rec.Body.Bytes(), &cmds)).To(Succeed())
			Expect(cmds).To(HaveLen(3))
			Expect(cmds[0].BatchID).NotTo(BeEmpty())
			Expect(cmds[1].BatchID).To(Equal(cmds[0].BatchID))
			Expect(cmds[0].FailFast).To(BeTrue())
		})

		It("leaves a single-node command outside any batch", func() {
			body := `{"command":"upgrade"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")
			Expect(handler.Create(c)).To(Succeed())

			var cmd store.NodeCommand
			Expect(json.Unmarshal(rec.Body.Bytes(), &cmd)).To(Succeed())
			Expect(cmd.BatchID).To(BeEmpty())
			Expect(cmd.FailFast).To(BeFalse())
		})
	})

	Describe("fail-fast", func() {
		It("cancels the pending rest of the batch when a node fails", func() {
			cmds := postBulk(`{"selector":{"nodeIDs":["node-1","node-2","node-3"]},"command":"upgrade","failFast":true}`)

			reportStatus("node-1", cmds[0].ID, store.CommandFailed)

			Expect(find(cmds[0].ID).Phase).To(Equal(store.CommandFailed))
			Expect(find(cmds[1].ID).Phase).To(Equal(store.CommandCanceled))
			Expect(find(cmds[2].ID).Phase).To(Equal(store.CommandCanceled))
			Expect(find(cmds[1].ID).Result).To(ContainSubstring("node-1"))
			Expect(find(cmds[1].ID).CompletedAt).NotTo(BeNil())
		})

		// The cancellation boundary. pushCommand claims a command
		// Pending -> Delivered before it writes to the socket, and there is no
		// abort message in the protocol, so a delivered command is already the
		// node's. Cancelling it here would report a rollback that never happened.
		It("leaves an already delivered command alone", func() {
			cmds := postBulk(`{"selector":{"nodeIDs":["node-1","node-2","node-3"]},"command":"upgrade","failFast":true}`)
			find(cmds[1].ID).Phase = store.CommandRunning
			_, err := cs.ClaimForDelivery(context.Background(), cmds[2].ID)
			Expect(err).NotTo(HaveOccurred())

			reportStatus("node-1", cmds[0].ID, store.CommandFailed)

			Expect(find(cmds[1].ID).Phase).To(Equal(store.CommandRunning))
			Expect(find(cmds[2].ID).Phase).To(Equal(store.CommandDelivered))
		})

		It("does not touch the batch when fail-fast was not asked for", func() {
			cmds := postBulk(`{"selector":{"nodeIDs":["node-1","node-2","node-3"]},"command":"upgrade"}`)

			reportStatus("node-1", cmds[0].ID, store.CommandFailed)

			Expect(find(cmds[1].ID).Phase).To(Equal(store.CommandPending))
			Expect(find(cmds[2].ID).Phase).To(Equal(store.CommandPending))
		})

		It("does not touch the batch when a node succeeds", func() {
			cmds := postBulk(`{"selector":{"nodeIDs":["node-1","node-2","node-3"]},"command":"upgrade","failFast":true}`)

			reportStatus("node-1", cmds[0].ID, store.CommandCompleted)

			Expect(find(cmds[1].ID).Phase).To(Equal(store.CommandPending))
			Expect(find(cmds[2].ID).Phase).To(Equal(store.CommandPending))
		})

		It("does not let one batch's failure cancel another batch", func() {
			first := postBulk(`{"selector":{"nodeIDs":["node-1","node-2"]},"command":"upgrade","failFast":true}`)
			second := postBulk(`{"selector":{"nodeIDs":["node-3"]},"command":"upgrade","failFast":true}`)

			reportStatus("node-1", first[0].ID, store.CommandFailed)

			Expect(find(first[1].ID).Phase).To(Equal(store.CommandCanceled))
			Expect(find(second[0].ID).Phase).To(Equal(store.CommandPending))
		})

		// A single-node command carries no batch id, so a failure on it must not
		// reach for a batch and cancel every command that also has none.
		It("does not cancel unbatched commands when an unbatched one fails", func() {
			cs.cmds = []*store.NodeCommand{
				{ID: "solo-1", ManagedNodeID: "node-1", Phase: store.CommandDelivered},
				{ID: "solo-2", ManagedNodeID: "node-2", Phase: store.CommandPending},
			}

			reportStatus("node-1", "solo-1", store.CommandFailed)

			Expect(find("solo-2").Phase).To(Equal(store.CommandPending))
		})

		// The flag is the server's, not the reporting node's: a node that sends
		// failFast in its status body must not be able to stop a rollout that
		// was never created fail-fast.
		It("ignores a fail-fast claim made by the reporting node", func() {
			cmds := postBulk(`{"selector":{"nodeIDs":["node-1","node-2"]},"command":"upgrade"}`)
			body := `{"phase":"Failed","result":"boom","failFast":true,"batchID":"` + cmds[0].BatchID + `"}`
			req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/node-1/commands/"+cmds[0].ID+"/status", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID", "commandID")
			c.SetParamValues("node-1", cmds[0].ID)
			c.Set(auth.ContextKeyNodeID, "node-1")
			Expect(handler.UpdateStatus(c)).To(Succeed())

			Expect(find(cmds[1].ID).Phase).To(Equal(store.CommandPending))
		})

		It("stops the batch on an admin-written failure too", func() {
			cmds := postBulk(`{"selector":{"nodeIDs":["node-1","node-2"]},"command":"upgrade","failFast":true}`)
			body := `{"phase":"Failed","result":"operator marked it failed"}`
			req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/node-1/commands/"+cmds[0].ID+"/status", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID", "commandID")
			c.SetParamValues("node-1", cmds[0].ID)
			// No auth.ContextKeyNodeID: this is the admin branch.
			Expect(handler.UpdateStatus(c)).To(Succeed())

			Expect(find(cmds[1].ID).Phase).To(Equal(store.CommandCanceled))
		})
	})

	Describe("BatchStatus", func() {
		getBatch := func(batchID string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/batches/"+batchID, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("batchID")
			c.SetParamValues(batchID)
			ExpectWithOffset(1, handler.BatchStatus(c)).To(Succeed())
			return rec
		}

		It("reports a batch still in flight as Pending", func() {
			cmds := postBulk(`{"selector":{"nodeIDs":["node-1","node-2","node-3"]},"command":"upgrade","failFast":true}`)
			reportStatus("node-1", cmds[0].ID, store.CommandCompleted)

			rec := getBatch(cmds[0].BatchID)
			Expect(rec.Code).To(Equal(http.StatusOK))
			var out store.BatchOutcome
			Expect(json.Unmarshal(rec.Body.Bytes(), &out)).To(Succeed())
			Expect(out.Phase).To(Equal(store.CommandPending))
			Expect(out.Total).To(Equal(3))
			Expect(out.Completed).To(Equal(1))
			Expect(out.Pending).To(Equal(2))
			Expect(out.FailFast).To(BeTrue())
			Expect(out.Command).To(Equal("upgrade"))
		})

		It("reports a stopped batch as Failed and names the node that failed", func() {
			cmds := postBulk(`{"selector":{"nodeIDs":["node-1","node-2","node-3"]},"command":"upgrade","failFast":true}`)
			reportStatus("node-1", cmds[0].ID, store.CommandFailed)

			var out store.BatchOutcome
			Expect(json.Unmarshal(getBatch(cmds[0].BatchID).Body.Bytes(), &out)).To(Succeed())
			Expect(out.Phase).To(Equal(store.CommandFailed))
			Expect(out.Failed).To(Equal(1))
			Expect(out.Canceled).To(Equal(2))
			Expect(out.Pending).To(BeZero())
			Expect(out.Commands).To(HaveLen(3))
			Expect(out.Commands[0].ManagedNodeID).To(Equal("node-1"))
			Expect(out.Commands[0].Result).To(Equal("reported"))
		})

		It("reports a fully successful batch as Completed", func() {
			cmds := postBulk(`{"selector":{"nodeIDs":["node-1","node-2","node-3"]},"command":"upgrade"}`)
			for i, nodeID := range []string{"node-1", "node-2", "node-3"} {
				reportStatus(nodeID, cmds[i].ID, store.CommandCompleted)
			}

			var out store.BatchOutcome
			Expect(json.Unmarshal(getBatch(cmds[0].BatchID).Body.Bytes(), &out)).To(Succeed())
			Expect(out.Phase).To(Equal(store.CommandCompleted))
			Expect(out.Completed).To(Equal(3))
		})

		It("returns 404 for a batch that does not exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/batches/nope", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("batchID")
			c.SetParamValues("nope")
			Expect(handler.BatchStatus(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusNotFound))
		})
	})
})
