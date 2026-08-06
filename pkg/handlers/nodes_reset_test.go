package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/kairos-io/AuroraBoot/pkg/ws"
	"github.com/labstack/echo/v4"
)

var _ = Describe("Reset lifecycle", func() {
	var (
		e  *echo.Echo
		ns *fakeNodeStore
	)

	BeforeEach(func() {
		e = echo.New()
		ns = &fakeNodeStore{}
	})

	Describe("issuing a reset command marks the node pending", func() {
		var cmdHandler *handlers.CommandHandler

		BeforeEach(func() {
			ns.nodes = []*store.ManagedNode{{ID: "node-1"}}
			cmdHandler = handlers.NewCommandHandler(&fakeCommandStore{}, ns, nil)
		})

		create := func(body string) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")
			Expect(cmdHandler.Create(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusCreated))
		}

		It("sets ResetState=pending for a reset command", func() {
			create(`{"command":"reset"}`)
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStatePending))
			Expect(ns.nodes[0].ResetRequestedAt).NotTo(BeNil())
		})

		It("leaves ResetState empty for a non-reset command", func() {
			create(`{"command":"upgrade","args":{"version":"1.2.0"}}`)
			Expect(ns.nodes[0].ResetState).To(Equal(""))
		})
	})

	Describe("re-register resolves the reset from the reported boot state", func() {
		var nodeHandler *handlers.NodeHandler

		// reregister drives Register for an already-known machineID (existing node).
		reregister := func(bootState string) {
			body := `{"machineID":"m1","bootState":"` + bootState + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			Expect(nodeHandler.Register(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusOK)) // existing node
		}

		seed := func(resetState string) {
			ns.nodes = []*store.ManagedNode{{ID: "node-1", MachineID: "m1", APIKey: "k", ResetState: resetState}}
			nodeHandler = handlers.NewNodeHandler(ns, &fakeCommandStore{}, &fakeGroupStore{}, ws.NewHub(), "reg-token", "http://localhost:8080")
		}

		It("pending + active boot => done, stamps LastReset", func() {
			seed(store.ResetStatePending)
			reregister("active")
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateDone))
			Expect(ns.nodes[0].LastReset).NotTo(BeNil())
		})

		It("pending + autoreset boot => in-progress", func() {
			seed(store.ResetStatePending)
			reregister("autoreset")
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateInProgress))
			Expect(ns.nodes[0].LastReset).To(BeNil())
		})

		It("in-progress + active boot => done", func() {
			seed(store.ResetStateInProgress)
			reregister("active")
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateDone))
			Expect(ns.nodes[0].LastReset).NotTo(BeNil())
		})

		It("pending + passive boot => failed (active image did not survive)", func() {
			seed(store.ResetStatePending)
			reregister("passive")
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateFailed))
			Expect(ns.nodes[0].LastReset).To(BeNil())
		})

		It("pending + recovery boot => failed", func() {
			seed(store.ResetStatePending)
			reregister("recovery")
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateFailed))
		})

		It("leaves a node that is not awaiting a reset untouched", func() {
			seed("") // no reset in flight
			reregister("active")
			Expect(ns.nodes[0].ResetState).To(Equal(""))
			Expect(ns.nodes[0].LastReset).To(BeNil())
		})
	})
})
