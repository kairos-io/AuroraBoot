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

var _ = Describe("Upgrade lifecycle", func() {
	var (
		e  *echo.Echo
		ns *fakeNodeStore
	)

	BeforeEach(func() {
		e = echo.New()
		ns = &fakeNodeStore{}
	})

	Describe("issuing an upgrade command marks the node pending", func() {
		var cmdHandler *handlers.CommandHandler

		BeforeEach(func() {
			ns.nodes = []*store.ManagedNode{{ID: "node-1"}}
			cmdHandler = handlers.NewCommandHandler(&fakeCommandStore{}, ns, nil, nil, nil)
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

		It("sets UpgradeState=pending for an upgrade command", func() {
			create(`{"command":"upgrade","args":{"source":"oci:quay.io/kairos/opensuse:v4"}}`)
			Expect(ns.nodes[0].UpgradeState).To(Equal(store.UpgradeStatePending))
			Expect(ns.nodes[0].UpgradeRequestedAt).NotTo(BeNil())
			Expect(ns.nodes[0].ResetState).To(Equal(""))
		})

		It("leaves UpgradeState empty for a non-upgrade command", func() {
			create(`{"command":"reboot"}`)
			Expect(ns.nodes[0].UpgradeState).To(Equal(""))
		})

		It("leaves UpgradeState empty for upgrade-recovery, which does not reboot into the new image", func() {
			create(`{"command":"upgrade-recovery","args":{"source":"oci:quay.io/kairos/opensuse:v4","recovery":"true"}}`)
			Expect(ns.nodes[0].UpgradeState).To(Equal(""))
		})

		It("leaves UpgradeState empty for a reset command", func() {
			create(`{"command":"reset"}`)
			Expect(ns.nodes[0].UpgradeState).To(Equal(""))
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStatePending))
		})
	})

	Describe("issuing an upgrade via the bulk and group paths marks nodes pending", func() {
		var cmdHandler *handlers.CommandHandler

		BeforeEach(func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-1", GroupID: "grp-1"},
				{ID: "node-2", GroupID: "grp-1"},
			}
			cmdHandler = handlers.NewCommandHandler(&fakeCommandStore{}, ns, nil, nil, nil)
		})

		expectAllPending := func() {
			for _, n := range ns.nodes {
				Expect(n.UpgradeState).To(Equal(store.UpgradeStatePending))
				Expect(n.UpgradeRequestedAt).NotTo(BeNil())
			}
		}

		It("CreateBulk marks every selected node pending", func() {
			body := `{"selector":{"nodeIDs":["node-1","node-2"]},"command":"upgrade"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			Expect(cmdHandler.CreateBulk(e.NewContext(req, rec))).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusCreated))
			expectAllPending()
		})

		It("CreateForGroup marks every group node pending", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/grp-1/commands", strings.NewReader(`{"command":"upgrade"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("grp-1")
			Expect(cmdHandler.CreateForGroup(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusCreated))
			expectAllPending()
		})

		It("a non-upgrade bulk command leaves UpgradeState empty", func() {
			body := `{"selector":{"nodeIDs":["node-1","node-2"]},"command":"reboot"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/commands", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			Expect(cmdHandler.CreateBulk(e.NewContext(req, rec))).To(Succeed())
			for _, n := range ns.nodes {
				Expect(n.UpgradeState).To(Equal(""))
			}
		})

		It("a non-upgrade group command leaves UpgradeState empty", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/groups/grp-1/commands", strings.NewReader(`{"command":"reboot"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("grp-1")
			Expect(cmdHandler.CreateForGroup(c)).To(Succeed())
			for _, n := range ns.nodes {
				Expect(n.UpgradeState).To(Equal(""))
			}
		})
	})

	Describe("re-register resolves the upgrade from the reported boot state", func() {
		var nodeHandler *handlers.NodeHandler

		reregister := func(bootState string) {
			body := `{"machineID":"m1","bootState":"` + bootState + `"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			Expect(nodeHandler.Register(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusOK))
		}

		seed := func(upgradeState string) {
			ns.nodes = []*store.ManagedNode{{ID: "node-1", MachineID: "m1", APIKey: "k", UpgradeState: upgradeState}}
			nodeHandler = handlers.NewNodeHandler(ns, &fakeCommandStore{}, &fakeGroupStore{}, ws.NewHub(), "reg-token", "http://localhost:8080")
		}

		It("pending + active boot => done, stamps LastUpgrade", func() {
			seed(store.UpgradeStatePending)
			reregister("active")
			Expect(ns.nodes[0].UpgradeState).To(Equal(store.UpgradeStateDone))
			Expect(ns.nodes[0].LastUpgrade).NotTo(BeNil())
		})

		It("pending + passive boot => failed (the node fell back off the upgraded image)", func() {
			seed(store.UpgradeStatePending)
			reregister("passive")
			Expect(ns.nodes[0].UpgradeState).To(Equal(store.UpgradeStateFailed))
			Expect(ns.nodes[0].LastUpgrade).To(BeNil())
		})

		It("pending + recovery boot => failed", func() {
			seed(store.UpgradeStatePending)
			reregister("recovery")
			Expect(ns.nodes[0].UpgradeState).To(Equal(store.UpgradeStateFailed))
			Expect(ns.nodes[0].LastUpgrade).To(BeNil())
		})

		It("pending + no reported boot state stays pending", func() {
			seed(store.UpgradeStatePending)
			reregister("")
			Expect(ns.nodes[0].UpgradeState).To(Equal(store.UpgradeStatePending))
			Expect(ns.nodes[0].LastUpgrade).To(BeNil())
		})

		It("a finished upgrade is not resolved again", func() {
			seed(store.UpgradeStateFailed)
			reregister("active")
			Expect(ns.nodes[0].UpgradeState).To(Equal(store.UpgradeStateFailed))
			Expect(ns.nodes[0].LastUpgrade).To(BeNil())
		})

		It("leaves a node that is not awaiting an upgrade untouched", func() {
			seed("")
			reregister("active")
			Expect(ns.nodes[0].UpgradeState).To(Equal(""))
			Expect(ns.nodes[0].LastUpgrade).To(BeNil())
		})

		It("resolves the upgrade and a reset in flight on the same node independently", func() {
			seed(store.UpgradeStatePending)
			ns.nodes[0].ResetState = store.ResetStatePending
			reregister("active")
			Expect(ns.nodes[0].UpgradeState).To(Equal(store.UpgradeStateDone))
			Expect(ns.nodes[0].ResetState).To(Equal(store.ResetStateDone))
		})
	})
})
