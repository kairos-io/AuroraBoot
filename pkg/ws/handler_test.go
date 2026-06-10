package ws_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/kairos-io/AuroraBoot/pkg/ws"
	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var testCounter atomic.Int64

// wsMessage mirrors the envelope used by the handler.
type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type heartbeatData struct {
	AgentVersion string            `json:"agentVersion"`
	OSRelease    map[string]string `json:"osRelease,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

type commandData struct {
	ID      string            `json:"id"`
	Command string            `json:"command"`
	Args    map[string]string `json:"args,omitempty"`
}

type commandStatusData struct {
	ID     string `json:"id"`
	Phase  string `json:"phase"`
	Result string `json:"result,omitempty"`
}

func dialWS(server *httptest.Server, path string) (*websocket.Conn, *http.Response, error) {
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + path
	return websocket.DefaultDialer.Dial(wsURL, nil)
}

func sendMsg(conn *websocket.Conn, msgType string, data interface{}) error {
	raw, _ := json.Marshal(data)
	msg := wsMessage{Type: msgType, Data: raw}
	msgBytes, _ := json.Marshal(msg)
	return conn.WriteMessage(websocket.TextMessage, msgBytes)
}

func readMsg(conn *websocket.Conn) (*wsMessage, error) {
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	var msg wsMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

var _ = Describe("WebSocket Handler", func() {
	var (
		hub       *ws.Hub
		gormDB    *gormstore.Store
		nodes     store.NodeStore
		commands  store.CommandStore
		server    *httptest.Server
		nodeID    string
		apiKey    string
		finalized chan string
	)

	bg := context.Background()

	BeforeEach(func() {
		var err error
		// Every spec gets its own private, on-disk SQLite database in a
		// per-spec temp dir. We deliberately do NOT use a shared-cache
		// in-memory DSN (`mode=memory&cache=shared`): SQLite's shared cache
		// is process-wide, so every spec's in-memory DB would contend on a
		// single global write lock. A prior spec's still-draining read-loop
		// goroutine could then hold that lock long enough (up to the 5s
		// busy_timeout, retried) to starve the current spec's command_status
		// write past its Eventually deadline — the source of the flake.
		// A file-per-spec DB has no cross-spec lock contention.
		machineNum := testCounter.Add(1)
		dbName := filepath.Join(GinkgoT().TempDir(), fmt.Sprintf("ws_test_%d.db", machineNum))
		gormDB, err = gormstore.New(dbName)
		Expect(err).NotTo(HaveOccurred())

		nodes = &gormstore.NodeStoreAdapter{S: gormDB}
		commands = &gormstore.CommandStoreAdapter{S: gormDB}
		hub = ws.NewHub()

		// Create a test node. Register overwrites ID and APIKey.
		testNode := &store.ManagedNode{
			MachineID: fmt.Sprintf("machine-%d", machineNum),
			Hostname:  "test-host",
			Labels:    map[string]string{},
		}
		Expect(nodes.Register(bg, testNode)).To(Succeed())
		nodeID = testNode.ID
		apiKey = testNode.APIKey
		Expect(nodeID).NotTo(BeEmpty())
		Expect(apiKey).NotTo(BeEmpty())

		// Record auto eject-on-phone-home invocations: a WS heartbeat must fire
		// the same finalize hook as the REST register/heartbeat path.
		finalized = make(chan string, 8)
		agentHandler := &ws.AgentHandler{
			Hub:      hub,
			Nodes:    nodes,
			Commands: commands,
			Finalize: func(_ context.Context, id string) { finalized <- id },
		}
		uiHandler := &ws.UIHandler{Hub: hub}

		e := echo.New()
		e.GET("/api/v1/ws", agentHandler.HandleAgentWS)
		e.GET("/api/v1/ws/ui", uiHandler.HandleUIWS)
		server = httptest.NewServer(e)

		// Tear down in reverse: close the server first so it blocks until
		// every in-flight WS handler goroutine (and its read-loop) has
		// returned, THEN close the DB. Closing the DB while a read-loop is
		// still writing would race. DeferCleanup runs LIFO, so this is the
		// only teardown hook — no separate AfterEach.
		DeferCleanup(func() {
			server.Close()
			_ = gormDB.Close()
		})
	})

	Describe("Agent connection", func() {
		It("should reject connection without token", func() {
			_, resp, err := dialWS(server, "/api/v1/ws")
			Expect(err).To(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should reject connection with invalid token", func() {
			_, resp, err := dialWS(server, "/api/v1/ws?token=bad-token")
			Expect(err).To(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should accept connection with valid token and mark node online", func() {
			conn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			Eventually(func() bool {
				return hub.IsOnline(nodeID)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			Eventually(func() string {
				node, _ := nodes.GetByID(bg, nodeID)
				return node.Phase
			}, 5*time.Second, 100*time.Millisecond).Should(Equal(store.PhaseOnline))
		})

		It("should mark node offline when agent disconnects", func() {
			conn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				return hub.IsOnline(nodeID)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			conn.Close()

			Eventually(func() bool {
				return hub.IsOnline(nodeID)
			}, 10*time.Second, 100*time.Millisecond).Should(BeFalse())

			Eventually(func() string {
				node, _ := nodes.GetByID(bg, nodeID)
				if node == nil {
					return ""
				}
				return node.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(store.PhaseOffline))
		})

		It("should handle heartbeat and update node info", func() {
			conn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			Eventually(func() bool {
				return hub.IsOnline(nodeID)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			hb := heartbeatData{
				AgentVersion: "2.0.0",
				OSRelease:    map[string]string{"ID": "kairos"},
			}
			Expect(sendMsg(conn, "heartbeat", hb)).To(Succeed())

			Eventually(func() string {
				node, _ := nodes.GetByID(bg, nodeID)
				if node == nil {
					return ""
				}
				return node.AgentVersion
			}, 10*time.Second, 100*time.Millisecond).Should(Equal("2.0.0"))
		})

		It("should trigger the auto eject-on-phone-home finalizer on heartbeat", func() {
			conn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			Eventually(func() bool {
				return hub.IsOnline(nodeID)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			Expect(sendMsg(conn, "heartbeat", heartbeatData{AgentVersion: "2.0.0"})).To(Succeed())

			// The hook fires off the read loop with the node this connection
			// authenticated as — never an agent-chosen id.
			Eventually(finalized, 10*time.Second).Should(Receive(Equal(nodeID)))
		})

		It("should handle command_status and update command", func() {
			cmd := &store.NodeCommand{
				ManagedNodeID: nodeID,
				Command:       "upgrade",
			}
			Expect(commands.Create(bg, cmd)).To(Succeed())
			// Create sets Phase to Pending; mark as Delivered so it won't be sent on connect.
			Expect(commands.UpdateStatus(bg, cmd.ID, store.CommandDelivered, "")).To(Succeed())

			conn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			Eventually(func() bool {
				return hub.IsOnline(nodeID)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			status := commandStatusData{
				ID:     cmd.ID,
				Phase:  store.CommandCompleted,
				Result: "success",
			}
			Expect(sendMsg(conn, "command_status", status)).To(Succeed())

			Eventually(func() string {
				c, _ := commands.GetByID(bg, cmd.ID)
				if c == nil {
					return ""
				}
				return c.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(store.CommandCompleted))
		})

		It("should reject a command_status for a command owned by another node", func() {
			// Register a second node and queue a command for IT.
			otherNum := testCounter.Add(1000)
			other := &store.ManagedNode{
				MachineID: fmt.Sprintf("machine-other-%d", otherNum),
				Hostname:  "other-host",
				Labels:    map[string]string{},
			}
			Expect(nodes.Register(bg, other)).To(Succeed())

			foreign := &store.NodeCommand{
				ManagedNodeID: other.ID,
				Command:       "upgrade",
			}
			Expect(commands.Create(bg, foreign)).To(Succeed())
			Expect(commands.UpdateStatus(bg, foreign.ID, store.CommandDelivered, "")).To(Succeed())

			// Connect as the FIRST node and try to move the other node's command.
			conn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			Eventually(func() bool {
				return hub.IsOnline(nodeID)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			status := commandStatusData{
				ID:     foreign.ID,
				Phase:  store.CommandCompleted,
				Result: "pwned",
			}
			Expect(sendMsg(conn, "command_status", status)).To(Succeed())

			// The foreign command must stay Delivered — the impersonating
			// node's update is dropped. Consistently holds over the window.
			Consistently(func() string {
				c, _ := commands.GetByID(bg, foreign.ID)
				if c == nil {
					return ""
				}
				return c.Phase
			}, 1*time.Second, 100*time.Millisecond).Should(Equal(store.CommandDelivered))
		})

		It("should send pending commands on connect", func() {
			cmd := &store.NodeCommand{
				ManagedNodeID: nodeID,
				Command:       "reset",
				Args:          map[string]string{"force": "true"},
			}
			Expect(commands.Create(bg, cmd)).To(Succeed())
			// Create sets Phase to Pending, which is what we want.
			cmdID := cmd.ID

			conn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			msg, err := readMsg(conn)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg.Type).To(Equal("command"))

			var cd commandData
			Expect(json.Unmarshal(msg.Data, &cd)).To(Succeed())
			Expect(cd.ID).To(Equal(cmdID))
			Expect(cd.Command).To(Equal("reset"))
			Expect(cd.Args["force"]).To(Equal("true"))

			Eventually(func() string {
				c, _ := commands.GetByID(bg, cmdID)
				if c == nil {
					return ""
				}
				return c.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(store.CommandDelivered))
		})
	})

	Describe("Hub", func() {
		It("should send command to online node", func() {
			conn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			Eventually(func() bool {
				return hub.IsOnline(nodeID)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			cmdPayload := commandData{
				ID:      "cmd-3",
				Command: "exec",
				Args:    map[string]string{"script": "echo hello"},
			}
			Expect(hub.SendCommand(nodeID, cmdPayload)).To(Succeed())

			msg, err := readMsg(conn)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg.Type).To(Equal("command"))

			var cd commandData
			Expect(json.Unmarshal(msg.Data, &cd)).To(Succeed())
			Expect(cd.ID).To(Equal("cmd-3"))
			Expect(cd.Command).To(Equal("exec"))
		})

		It("should return error when sending command to offline node", func() {
			err := hub.SendCommand("nonexistent-node", map[string]string{"cmd": "test"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
		})

		It("should track online count correctly", func() {
			Expect(hub.OnlineCount()).To(Equal(0))

			conn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				return hub.OnlineCount()
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(1))

			conn.Close()

			Eventually(func() int {
				return hub.OnlineCount()
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(0))
		})
	})

	Describe("UI WebSocket", func() {
		It("should accept UI WebSocket connections", func() {
			conn, _, err := dialWS(server, "/api/v1/ws/ui")
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()
			Expect(conn).NotTo(BeNil())
		})
	})

	Describe("UIHub", func() {
		It("should broadcast command_update to connected UI clients", func() {
			// Connect a UI WS client.
			uiConn, _, err := dialWS(server, "/api/v1/ws/ui")
			Expect(err).NotTo(HaveOccurred())
			defer uiConn.Close()

			// Give the UI connection time to register.
			time.Sleep(100 * time.Millisecond)

			// Create a command and connect agent.
			cmd := &store.NodeCommand{
				ManagedNodeID: nodeID,
				Command:       "upgrade",
			}
			Expect(commands.Create(bg, cmd)).To(Succeed())
			Expect(commands.UpdateStatus(bg, cmd.ID, store.CommandDelivered, "")).To(Succeed())

			agentConn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())
			defer agentConn.Close()

			Eventually(func() bool {
				return hub.IsOnline(nodeID)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			// Agent sends command_status.
			status := commandStatusData{
				ID:     cmd.ID,
				Phase:  store.CommandCompleted,
				Result: "upgrade done",
			}
			Expect(sendMsg(agentConn, "command_status", status)).To(Succeed())

			// UI client should receive a command_update broadcast.
			msg, err := readMsg(uiConn)
			Expect(err).NotTo(HaveOccurred())
			Expect(msg.Type).To(Equal("command_update"))

			var update commandStatusData
			Expect(json.Unmarshal(msg.Data, &update)).To(Succeed())
			Expect(update.ID).To(Equal(cmd.ID))
			Expect(update.Phase).To(Equal(store.CommandCompleted))
		})

		It("should not crash when no UI clients connected", func() {
			// Create a command and connect agent only (no UI client).
			cmd := &store.NodeCommand{
				ManagedNodeID: nodeID,
				Command:       "exec",
			}
			Expect(commands.Create(bg, cmd)).To(Succeed())
			Expect(commands.UpdateStatus(bg, cmd.ID, store.CommandDelivered, "")).To(Succeed())

			agentConn, _, err := dialWS(server, "/api/v1/ws?token="+apiKey)
			Expect(err).NotTo(HaveOccurred())
			defer agentConn.Close()

			Eventually(func() bool {
				return hub.IsOnline(nodeID)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			// Agent sends command_status — should not panic.
			status := commandStatusData{
				ID:     cmd.ID,
				Phase:  store.CommandRunning,
				Result: "in progress",
			}
			Expect(sendMsg(agentConn, "command_status", status)).To(Succeed())

			// Verify the command was updated in the DB (handler didn't crash).
			Eventually(func() string {
				c, _ := commands.GetByID(bg, cmd.ID)
				if c == nil {
					return ""
				}
				return c.Phase
			}, 10*time.Second, 100*time.Millisecond).Should(Equal(store.CommandRunning))
		})
	})
})
