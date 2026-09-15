package handlers_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/auth"
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
		var (
			cmdHandler *handlers.CommandHandler
			cs         *fakeCommandStore
		)

		BeforeEach(func() {
			ns.nodes = []*store.ManagedNode{{ID: "node-1"}}
			cs = &fakeCommandStore{}
			cmdHandler = handlers.NewCommandHandler(cs, ns, nil)
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
			Expect(cs.cmds).To(HaveLen(1))
			Expect(cs.cmds[0].ExpiresAt).NotTo(BeNil())
			Expect(time.Until(*cs.cmds[0].ExpiresAt)).To(BeNumerically("~", handlers.DefaultResetTimeout, time.Second))
		})

		It("leaves ResetState empty for a non-reset command", func() {
			create(`{"command":"upgrade","args":{"version":"1.2.0"}}`)
			Expect(ns.nodes[0].ResetState).To(Equal(""))
			Expect(cs.cmds[0].ExpiresAt).To(BeNil())
		})
	})

	Describe("issuing a reset via the bulk and group paths marks nodes pending", func() {
		var (
			cmdHandler *handlers.CommandHandler
			cs         *fakeCommandStore
		)

		BeforeEach(func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-1", GroupID: "grp-1"},
				{ID: "node-2", GroupID: "grp-1"},
			}
			cs = &fakeCommandStore{}
			cmdHandler = handlers.NewCommandHandler(cs, ns, nil)
		})

		expectAllPending := func() {
			for _, n := range ns.nodes {
				Expect(n.ResetState).To(Equal(store.ResetStatePending))
				Expect(n.ResetRequestedAt).NotTo(BeNil())
			}
			for _, cmd := range cs.cmds {
				Expect(cmd.ExpiresAt).NotTo(BeNil())
			}
		}

		It("CreateBulk marks every selected node pending", func() {
			body := `{"selector":{"nodeIDs":["node-1","node-2"]},"command":"reset"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			Expect(cmdHandler.CreateBulk(e.NewContext(req, rec))).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusCreated))
			expectAllPending()
		})

		It("CreateForGroup marks every group node pending", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/grp-1/commands", strings.NewReader(`{"command":"reset"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("grp-1")
			Expect(cmdHandler.CreateForGroup(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusCreated))
			expectAllPending()
		})

		It("a non-reset bulk command leaves ResetState empty", func() {
			body := `{"selector":{"nodeIDs":["node-1","node-2"]},"command":"upgrade"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			Expect(cmdHandler.CreateBulk(e.NewContext(req, rec))).To(Succeed())
			for _, n := range ns.nodes {
				Expect(n.ResetState).To(Equal(""))
			}
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

	Describe("reading nodes expires stale resets", func() {
		var nodeHandler *handlers.NodeHandler

		BeforeEach(func() {
			nodeHandler = handlers.NewNodeHandler(ns, &fakeCommandStore{}, &fakeGroupStore{}, ws.NewHub(), "reg-token", "http://localhost:8080").
				WithResetTimeout(30 * time.Minute)
		})

		It("marks an overdue pending reset failed when listing nodes", func() {
			requestedAt := time.Now().Add(-31 * time.Minute)
			ns.nodes = []*store.ManagedNode{{
				ID:               "node-1",
				ResetState:       store.ResetStatePending,
				ResetRequestedAt: &requestedAt,
			}}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
			rec := httptest.NewRecorder()
			Expect(nodeHandler.List(e.NewContext(req, rec))).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateFailed))
		})

		It("marks an overdue in-progress reset failed when getting a node", func() {
			requestedAt := time.Now().Add(-31 * time.Minute)
			ns.nodes = []*store.ManagedNode{{
				ID:               "node-1",
				ResetState:       store.ResetStateInProgress,
				ResetRequestedAt: &requestedAt,
			}}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")
			Expect(nodeHandler.Get(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateFailed))
		})

		It("leaves a recent reset in progress", func() {
			requestedAt := time.Now().Add(-29 * time.Minute)
			ns.nodes = []*store.ManagedNode{{
				ID:               "node-1",
				ResetState:       store.ResetStatePending,
				ResetRequestedAt: &requestedAt,
			}}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
			rec := httptest.NewRecorder()
			Expect(nodeHandler.List(e.NewContext(req, rec))).To(Succeed())
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStatePending))
		})

		// Expiry is bookkeeping on a read path the UI and the CAPI provider both
		// poll, so a rejected write must not hide the fleet. Same outcome as a
		// lost CAS: 200 with the state the snapshot holds.
		It("still returns the node list when the expiry transition cannot be persisted", func() {
			requestedAt := time.Now().Add(-31 * time.Minute)
			ns.nodes = []*store.ManagedNode{{
				ID:               "node-1",
				ResetState:       store.ResetStatePending,
				ResetRequestedAt: &requestedAt,
			}}
			ns.failResetBeforeErr = errors.New("write failed")

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
			rec := httptest.NewRecorder()
			Expect(nodeHandler.List(e.NewContext(req, rec))).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(ContainSubstring("node-1"))
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStatePending))
		})

		It("still returns the node when a single-node read cannot persist the expiry", func() {
			requestedAt := time.Now().Add(-31 * time.Minute)
			ns.nodes = []*store.ManagedNode{{
				ID:               "node-1",
				ResetState:       store.ResetStateInProgress,
				ResetRequestedAt: &requestedAt,
			}}
			ns.failResetBeforeErr = errors.New("write failed")

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")
			Expect(nodeHandler.Get(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateInProgress))
		})

		// kairos-io/kairos#4287 asks to fail a reset only when the node has not
		// come back since it was requested. A node that re-registered mid-reset is
		// still wiping, so the original request window elapsing must not fail it.
		It("leaves a reset in progress when the node re-registered after the deadline", func() {
			requestedAt := time.Now().Add(-31 * time.Minute)
			progressAt := time.Now().Add(-1 * time.Minute)
			ns.nodes = []*store.ManagedNode{{
				ID:               "node-1",
				ResetState:       store.ResetStateInProgress,
				ResetRequestedAt: &requestedAt,
				ResetProgressAt:  &progressAt,
			}}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
			rec := httptest.NewRecorder()
			Expect(nodeHandler.List(e.NewContext(req, rec))).To(Succeed())
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateInProgress))
			// The handler must not even attempt the transition: the store's CAS is
			// the second line of defence, not the one under test here.
			Expect(ns.failResetBeforeCalls).To(BeZero())
		})

		It("expires a reset whose re-register is itself older than the deadline", func() {
			requestedAt := time.Now().Add(-90 * time.Minute)
			progressAt := time.Now().Add(-31 * time.Minute)
			ns.nodes = []*store.ManagedNode{{
				ID:               "node-1",
				ResetState:       store.ResetStateInProgress,
				ResetRequestedAt: &requestedAt,
				ResetProgressAt:  &progressAt,
			}}

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
			rec := httptest.NewRecorder()
			Expect(nodeHandler.List(e.NewContext(req, rec))).To(Succeed())
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateFailed))
		})
	})

	Describe("a node re-registering mid-reset re-anchors the expiry budget", func() {
		It("stamps ResetProgressAt on the pending to in-progress transition", func() {
			ns.nodes = []*store.ManagedNode{{
				ID: "node-1", MachineID: "m1", APIKey: "k",
				ResetState: store.ResetStatePending,
			}}
			nodeHandler := handlers.NewNodeHandler(ns, &fakeCommandStore{}, &fakeGroupStore{}, ws.NewHub(), "reg-token", "http://localhost:8080")

			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", strings.NewReader(`{"machineID":"m1","bootState":"autoreset"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			Expect(nodeHandler.Register(e.NewContext(req, rec))).To(Succeed())

			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateInProgress))
			Expect(ns.nodes[0].ResetProgressAt).NotTo(BeNil())
			Expect(time.Since(*ns.nodes[0].ResetProgressAt)).To(BeNumerically("<", time.Minute))
		})

		It("does not stamp ResetProgressAt on the terminal transitions", func() {
			for _, bootState := range []string{store.BootStateActive, store.BootStatePassive, store.BootStateRecovery} {
				ns = &fakeNodeStore{nodes: []*store.ManagedNode{{
					ID: "node-1", MachineID: "m1", APIKey: "k",
					ResetState: store.ResetStatePending,
				}}}
				nodeHandler := handlers.NewNodeHandler(ns, &fakeCommandStore{}, &fakeGroupStore{}, ws.NewHub(), "reg-token", "http://localhost:8080")

				req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", strings.NewReader(`{"machineID":"m1","bootState":"`+bootState+`"}`))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				Expect(nodeHandler.Register(e.NewContext(req, rec))).To(Succeed())
				Expect(ns.nodes[0].ResetProgressAt).To(BeNil(), "bootState %s", bootState)
			}
		})

		It("clears ResetProgressAt when a new reset is requested", func() {
			progressAt := time.Now().Add(-2 * time.Hour)
			ns.nodes = []*store.ManagedNode{{ID: "node-1", ResetProgressAt: &progressAt}}
			cs := &fakeCommandStore{}
			cmdHandler := handlers.NewCommandHandler(cs, ns, nil)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/commands", strings.NewReader(`{"command":"reset"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")
			Expect(cmdHandler.Create(c)).To(Succeed())

			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStatePending))
			Expect(ns.nodes[0].ResetProgressAt).To(BeNil())
		})
	})

	// The store gates both GetPending and ClaimForDelivery on expires_at, so a
	// node that comes back after its reset lifecycle failed must not be handed the
	// stale reset command.
	Describe("an expired reset command is not delivered", func() {
		It("omits it from the agent poll and leaves it pending", func() {
			expired := time.Now().Add(-time.Minute)
			ns.nodes = []*store.ManagedNode{{ID: "node-1"}}
			cs := &fakeCommandStore{cmds: []*store.NodeCommand{{
				ID:            "cmd-1",
				ManagedNodeID: "node-1",
				Command:       "reset",
				Phase:         store.CommandPending,
				ExpiresAt:     &expired,
			}}}
			nodeHandler := handlers.NewNodeHandler(ns, cs, &fakeGroupStore{}, ws.NewHub(), "reg-token", "http://localhost:8080")

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/commands", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")
			c.Set(auth.ContextKeyNodeID, "node-1")

			Expect(nodeHandler.GetCommands(c)).To(Succeed())

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).NotTo(ContainSubstring("cmd-1"))
			Expect(cs.cmds[0].Phase).To(Equal(store.CommandPending))
		})

		It("still delivers an unexpired one", func() {
			future := time.Now().Add(time.Hour)
			ns.nodes = []*store.ManagedNode{{ID: "node-1"}}
			cs := &fakeCommandStore{cmds: []*store.NodeCommand{{
				ID:            "cmd-1",
				ManagedNodeID: "node-1",
				Command:       "reset",
				Phase:         store.CommandPending,
				ExpiresAt:     &future,
			}}}
			nodeHandler := handlers.NewNodeHandler(ns, cs, &fakeGroupStore{}, ws.NewHub(), "reg-token", "http://localhost:8080")

			req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/commands", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("nodeID")
			c.SetParamValues("node-1")
			c.Set(auth.ContextKeyNodeID, "node-1")

			Expect(nodeHandler.GetCommands(c)).To(Succeed())

			Expect(rec.Body.String()).To(ContainSubstring("cmd-1"))
			Expect(cs.cmds[0].Phase).To(Equal(store.CommandDelivered))
		})
	})
})
