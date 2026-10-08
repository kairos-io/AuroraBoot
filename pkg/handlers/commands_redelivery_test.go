package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/kairos-io/AuroraBoot/pkg/ws"
	"github.com/labstack/echo/v4"
)

// deadConn registers a real WebSocket connection for nodeID on hub and then
// closes it, which is the state the push path can observe for real: the node
// drops between the hub's IsOnline check and the write. IsOnline keeps
// reporting true because the map entry is still there, while writeMessage
// fails with "use of closed network connection".
func deadConn(hub *ws.Hub, nodeID string) {
	serverConns := make(chan *websocket.Conn, 1)
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConns <- c
	}))
	DeferCleanup(srv.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/", nil)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(client.Close)

	var serverConn *websocket.Conn
	Eventually(serverConns).Should(Receive(&serverConn))
	hub.Register(nodeID, serverConn)
	Expect(serverConn.Close()).To(Succeed())
	Expect(hub.IsOnline(nodeID)).To(BeTrue())
	Expect(hub.SendCommand(nodeID, map[string]string{"probe": "1"})).To(HaveOccurred())
}

var _ = Describe("Command delivery when the WebSocket write fails", func() {
	var (
		e   *echo.Echo
		ns  *fakeNodeStore
		cs  *fakeCommandStore
		hub *ws.Hub
	)

	BeforeEach(func() {
		e = echo.New()
		ns = &fakeNodeStore{nodes: []*store.ManagedNode{{ID: "node-1", GroupID: "grp-1"}}}
		cs = &fakeCommandStore{}
		hub = ws.NewHub()
	})

	It("leaves a command that was never sent claimable by the next poll", func() {
		deadConn(hub, "node-1")
		handler := handlers.NewCommandHandler(cs, ns, hub, nil, nil)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/commands",
			strings.NewReader(`{"command":"upgrade","args":{"version":"1.2.0"}}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("nodeID")
		c.SetParamValues("node-1")

		Expect(handler.Create(c)).To(Succeed())
		Expect(rec.Code).To(Equal(http.StatusCreated))

		// The agent never received the push, so the command has to stay in the
		// queue the agent actually reads. GetPending matches Pending only, and
		// nothing else ever moves a command back out of Delivered, so a command
		// left Delivered here is a command that never runs.
		pending, err := cs.GetPending(c.Request().Context(), "node-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(pending).To(HaveLen(1), "the undelivered command must still be pending")
		Expect(pending[0].Command).To(Equal("upgrade"))
		Expect(pending[0].DeliveredAt).To(BeNil())
	})

	It("leaves an undelivered unregister command claimable by the next poll", func() {
		deadConn(hub, "node-1")
		handler := handlers.NewNodeHandler(ns, cs, &fakeGroupStore{}, hub, "", "")

		req := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/decommission", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("nodeID")
		c.SetParamValues("node-1")

		Expect(handler.Decommission(c)).To(Succeed())
		Expect(rec.Code).To(Equal(http.StatusOK))

		pending, err := cs.GetPending(c.Request().Context(), "node-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(pending).To(HaveLen(1), "the undelivered unregister command must still be pending")
		Expect(pending[0].Command).To(Equal("unregister"))
	})
})
