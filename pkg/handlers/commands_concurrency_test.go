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

// A batch with a concurrency limit paces itself: only so many nodes hold a
// command at once, and the next one is released when an earlier one reports
// (kairos-io/kairos#4267, acceptance criterion 2). These specs pin what the
// limit does to the rows a fan-out writes and what a node's report does to the
// rows that are still waiting.
var _ = Describe("CommandHandler batch concurrency", func() {
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
				{ID: "node-1", GroupID: new("grp-1")},
				{ID: "node-2", GroupID: new("grp-1")},
				{ID: "node-3", GroupID: new("grp-1")},
			},
		}
		cs = &fakeCommandStore{}
		handler = handlers.NewCommandHandler(cs, ns, nil, nil, nil)
	})

	postBulk := func(body string) (*httptest.ResponseRecorder, []*store.NodeCommand) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/commands", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		ExpectWithOffset(1, handler.CreateBulk(e.NewContext(req, rec))).To(Succeed())
		if rec.Code != http.StatusCreated {
			return rec, nil
		}
		var cmds []*store.NodeCommand
		ExpectWithOffset(1, json.Unmarshal(rec.Body.Bytes(), &cmds)).To(Succeed())
		return rec, cmds
	}

	reportStatus := func(nodeID, commandID, phase string) {
		body := `{"phase":"` + phase + `","result":"reported"}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/nodes/"+nodeID+"/commands/"+commandID+"/status", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("nodeID", "commandID")
		c.SetParamValues(nodeID, commandID)
		c.Set(auth.ContextKeyNodeID, nodeID)
		ExpectWithOffset(1, handler.UpdateStatus(c)).To(Succeed())
		ExpectWithOffset(1, rec.Code).To(Equal(http.StatusOK))
	}

	phases := func() []string {
		out := make([]string, 0, len(cs.cmds))
		for _, cmd := range cs.cmds {
			out = append(out, cmd.Phase)
		}
		return out
	}

	It("dispatches to every node at once when no limit is asked for", func() {
		_, cmds := postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade"}`)
		Expect(cmds).To(HaveLen(3))
		Expect(phases()).To(Equal([]string{
			store.CommandPending, store.CommandPending, store.CommandPending,
		}))
		for _, cmd := range cmds {
			Expect(cmd.MaxConcurrency).To(BeZero())
		}
	})

	It("offers the command to maxConcurrency nodes and queues the rest", func() {
		_, cmds := postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade","maxConcurrency":1}`)
		Expect(cmds).To(HaveLen(3))
		Expect(phases()).To(Equal([]string{
			store.CommandPending, store.CommandQueued, store.CommandQueued,
		}))
	})

	It("writes every node of a throttled batch in the one CreateBatch call", func() {
		postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade","maxConcurrency":1}`)
		// A queued node that only existed once its turn came could not be
		// cancelled by fail-fast and would be missing from the batch status.
		Expect(cs.batches).To(HaveLen(1))
		Expect(cs.batches[0]).To(HaveLen(3))
	})

	It("carries the limit on every row of the batch, queued or not", func() {
		_, cmds := postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade","maxConcurrency":2}`)
		for _, cmd := range cmds {
			Expect(cmd.MaxConcurrency).To(Equal(2))
		}
		Expect(phases()).To(Equal([]string{
			store.CommandPending, store.CommandPending, store.CommandQueued,
		}))
	})

	It("releases the next node when the one holding the slot completes", func() {
		_, cmds := postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade","maxConcurrency":1}`)
		reportStatus("node-1", cmds[0].ID, store.CommandCompleted)
		Expect(phases()).To(Equal([]string{
			store.CommandCompleted, store.CommandPending, store.CommandQueued,
		}))
	})

	It("releases one node per report, never the whole queue at once", func() {
		_, cmds := postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade","maxConcurrency":1}`)
		reportStatus("node-1", cmds[0].ID, store.CommandCompleted)
		reportStatus("node-2", cmds[1].ID, store.CommandCompleted)
		Expect(phases()).To(Equal([]string{
			store.CommandCompleted, store.CommandCompleted, store.CommandPending,
		}))
	})

	It("releases a slot a failed node held when the batch is not fail-fast", func() {
		_, cmds := postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade","maxConcurrency":1}`)
		reportStatus("node-1", cmds[0].ID, store.CommandFailed)
		Expect(phases()).To(Equal([]string{
			store.CommandFailed, store.CommandPending, store.CommandQueued,
		}))
	})

	It("cancels the queued nodes of a fail-fast batch instead of releasing them", func() {
		_, cmds := postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade","maxConcurrency":1,"failFast":true}`)
		reportStatus("node-1", cmds[0].ID, store.CommandFailed)
		Expect(phases()).To(Equal([]string{
			store.CommandFailed, store.CommandCanceled, store.CommandCanceled,
		}))
	})

	It("does not release a slot while the node is only running", func() {
		_, cmds := postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade","maxConcurrency":1}`)
		reportStatus("node-1", cmds[0].ID, store.CommandRunning)
		Expect(phases()).To(Equal([]string{
			store.CommandRunning, store.CommandQueued, store.CommandQueued,
		}))
	})

	It("rejects a negative limit on the selector endpoint", func() {
		rec, _ := postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade","maxConcurrency":-1}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(cs.cmds).To(BeEmpty())
	})

	It("applies the limit to the group endpoint too", func() {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/grp-1/commands",
			strings.NewReader(`{"command":"upgrade","maxConcurrency":1}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("grp-1")
		Expect(handler.CreateForGroup(c)).To(Succeed())
		Expect(rec.Code).To(Equal(http.StatusCreated))
		Expect(phases()).To(Equal([]string{
			store.CommandPending, store.CommandQueued, store.CommandQueued,
		}))
	})

	It("rejects a negative limit on the group endpoint", func() {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/grp-1/commands",
			strings.NewReader(`{"command":"upgrade","maxConcurrency":-2}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("grp-1")
		Expect(handler.CreateForGroup(c)).To(Succeed())
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(cs.cmds).To(BeEmpty())
	})

	It("reports the queued nodes in the batch status", func() {
		_, cmds := postBulk(`{"selector":{"groupID":"grp-1"},"command":"upgrade","maxConcurrency":1}`)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/commands/batches/"+cmds[0].BatchID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("batchID")
		c.SetParamValues(cmds[0].BatchID)
		Expect(handler.BatchStatus(c)).To(Succeed())
		Expect(rec.Code).To(Equal(http.StatusOK))

		var out store.BatchOutcome
		Expect(json.Unmarshal(rec.Body.Bytes(), &out)).To(Succeed())
		Expect(out.Total).To(Equal(3))
		Expect(out.Queued).To(Equal(2))
		Expect(out.Pending).To(Equal(3))
		Expect(out.MaxConcurrency).To(Equal(1))
		Expect(out.Phase).To(Equal(store.CommandPending))
	})
})
