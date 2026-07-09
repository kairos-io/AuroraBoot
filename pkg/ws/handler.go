package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// wsMessage is the envelope for all WebSocket messages.
type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// heartbeatData is sent by the agent.
type heartbeatData struct {
	AgentVersion string            `json:"agentVersion"`
	OSRelease    map[string]string `json:"osRelease,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// commandData is sent to the agent.
type commandData struct {
	ID      string            `json:"id"`
	Command string            `json:"command"`
	Args    map[string]string `json:"args,omitempty"`
}

// commandStatusData is sent by the agent to report command status.
type commandStatusData struct {
	ID     string `json:"id"`
	Phase  string `json:"phase"`
	Result string `json:"result,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// finalizeTimeout bounds one auto eject-on-phone-home attempt fired from a WS
// heartbeat, mirroring the REST handlers' budget (pkg/handlers/finalize.go).
const finalizeTimeout = 2 * time.Minute

// AgentHandler handles WebSocket connections from agents.
type AgentHandler struct {
	Hub      *Hub
	Nodes    store.NodeStore
	Commands store.CommandStore

	// Finalize, when set, is the auto eject-on-phone-home hook (the same
	// MaybeFinalizeForNode wired into the REST Register/Heartbeat handlers). A WS
	// heartbeat is just as much an "OS is up" signal as a REST one, so it must
	// trigger the pending-eject finalize too — otherwise a node that reports
	// liveness only over the agent channel never gets its media ejected.
	Finalize func(ctx context.Context, nodeID string)
	// BaseCtx is the server lifecycle context the finalize goroutine derives from
	// (cancelled on shutdown). Nil means context.Background().
	BaseCtx context.Context

	// OnCommandStatus is invoked after the agent's command-status report has
	// been persisted. The server uses this hook to update node_extensions
	// tracking for the new `extension` command and for compound `upgrade`s
	// that carry an extensions[] payload. nil-safe — if unset, only the
	// command-status row is updated. Wired in pkg/server/server.go.
	OnCommandStatus func(ctx context.Context, nodeID string, cmd *store.NodeCommand)
}

// triggerFinalize fires the auto eject-on-phone-home hook off the WS read loop so
// it never blocks message handling. Nil-safe; the hook's per-deployment CAS makes
// repeated heartbeats harmless no-ops.
func (h *AgentHandler) triggerFinalize(nodeID string) {
	if h.Finalize == nil || nodeID == "" {
		return
	}
	base := h.BaseCtx
	if base == nil {
		base = context.Background()
	}
	go func() {
		ctx, cancel := context.WithTimeout(base, finalizeTimeout)
		defer cancel()
		h.Finalize(ctx, nodeID)
	}()
}

// HandleAgentWS handles GET /api/v1/ws?token=<apiKey>.
func (h *AgentHandler) HandleAgentWS(c echo.Context) error {
	token := c.QueryParam("token")
	if token == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing token"})
	}

	ctx := c.Request().Context()
	node, err := h.Nodes.GetByAPIKey(ctx, token)
	if err != nil || node == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
	}

	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Register in hub and mark node online. wc wraps conn with a per-connection
	// write lock; all writes to this connection (here and from the hub) must go
	// through it so they don't race.
	wc := h.Hub.Register(node.ID, conn)
	if err := h.Nodes.UpdatePhase(ctx, node.ID, store.PhaseOnline); err != nil {
		log.Printf("ws: failed to update node phase: %v", err)
	}

	defer func() {
		h.Hub.Unregister(node.ID)
		// context.Background() is correct here: the HTTP request context is
		// already done when the WS connection tears down, but we still need
		// to persist the offline phase change.
		if err := h.Nodes.UpdatePhase(context.Background(), node.ID, store.PhaseOffline); err != nil {
			log.Printf("ws: failed to update node phase: %v", err)
		}
	}()

	// Send pending commands on connect.
	h.sendPendingCommands(node.ID, wc)

	// Read loop.
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read error for node %s: %v", node.ID, err)
			}
			return nil
		}

		var msg wsMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("ws invalid message from node %s: %v", node.ID, err)
			continue
		}

		switch msg.Type {
		case "heartbeat":
			h.handleHeartbeat(node.ID, msg.Data)
		case "command_status":
			h.handleCommandStatus(node.ID, msg.Data)
		default:
			log.Printf("ws unknown message type from node %s: %s", node.ID, msg.Type)
		}
	}
}

func (h *AgentHandler) handleHeartbeat(nodeID string, data json.RawMessage) {
	var hb heartbeatData
	if err := json.Unmarshal(data, &hb); err != nil {
		log.Printf("ws invalid heartbeat from node %s: %v", nodeID, err)
		return
	}

	// context.Background() is correct here: handleHeartbeat is called from the
	// WS read loop which outlives the original HTTP request context.
	ctx := context.Background()
	// The WebSocket heartbeat does not carry network addresses or boot state
	// (those ride the REST register/heartbeat contract); pass nil/"" so the store
	// preserves whatever the node reported there.
	if err := h.Nodes.UpdateHeartbeat(ctx, nodeID, hb.AgentVersion, hb.OSRelease, nil, ""); err != nil {
		log.Printf("ws: failed to update heartbeat for node %s: %v", nodeID, err)
	}
	if err := h.Nodes.UpdatePhase(ctx, nodeID, store.PhaseOnline); err != nil {
		log.Printf("ws: failed to update node phase: %v", err)
	}
	// A WS heartbeat is an "OS is up" signal exactly like the REST heartbeat:
	// attempt the auto eject-on-phone-home (nil-safe, off this goroutine).
	h.triggerFinalize(nodeID)
}

// handleCommandStatus applies a command_status report from the agent. The
// update is scoped to nodeID — the node this WS connection authenticated as —
// so a node cannot move another node's command by sending its id. A miss
// (foreign or unknown command) is rejected without broadcasting.
func (h *AgentHandler) handleCommandStatus(nodeID string, data json.RawMessage) {
	var status commandStatusData
	if err := json.Unmarshal(data, &status); err != nil {
		log.Printf("ws invalid command_status: %v", err)
		return
	}

	// context.Background() is correct here: handleCommandStatus is called from
	// the WS read loop which outlives the original HTTP request context.
	ctx := context.Background()
	if err := h.Commands.UpdateStatusForNode(ctx, status.ID, nodeID, status.Phase, status.Result); err != nil {
		if errors.Is(err, store.ErrCommandNotFound) {
			log.Printf("ws: node %s attempted to update command %s it does not own; rejected", nodeID, status.ID)
		} else {
			log.Printf("ws: failed to update command status for %s: %v", status.ID, err)
		}
		return
	}
	if status.Phase == store.CommandCompleted && h.OnCommandStatus != nil {
		if cmd, err := h.Commands.GetByID(ctx, status.ID); err == nil && cmd != nil {
			h.OnCommandStatus(ctx, cmd.ManagedNodeID, cmd)
		}
	}

	if h.Hub != nil && h.Hub.UI != nil {
		h.Hub.UI.Broadcast(wsMessage{
			Type: "command_update",
			Data: data,
		})
	}
}

func (h *AgentHandler) sendPendingCommands(nodeID string, conn *wsConn) {
	ctx := context.Background()
	cmds, err := h.Commands.GetPending(ctx, nodeID)
	if err != nil || len(cmds) == 0 {
		return
	}

	for _, cmd := range cmds {
		// Atomically claim Pending→Delivered before sending so a concurrent REST
		// poll or WS push can't also deliver the same command. If another path
		// already claimed it, our claim returns false and we skip it — exactly-once
		// delivery, consistent with the poll path in NodeHandler.GetCommands.
		claimed, err := h.Commands.ClaimForDelivery(ctx, cmd.ID)
		if err != nil {
			log.Printf("ws: failed to claim pending command %s for node %s: %v", cmd.ID, nodeID, err)
			continue
		}
		if !claimed {
			continue
		}

		cmdMsg := commandData{
			ID:      cmd.ID,
			Command: cmd.Command,
			Args:    cmd.Args,
		}
		data, err := json.Marshal(cmdMsg)
		if err != nil {
			log.Printf("ws: failed to marshal command %s: %v", cmd.ID, err)
			continue
		}
		msg := wsMessage{Type: "command", Data: data}
		msgBytes, err := json.Marshal(msg)
		if err != nil {
			log.Printf("ws: failed to marshal ws message for command %s: %v", cmd.ID, err)
			continue
		}

		if err := conn.writeMessage(websocket.TextMessage, msgBytes); err != nil {
			log.Printf("ws: failed to send pending command %s to node %s: %v", cmd.ID, nodeID, err)
			break
		}
	}
}

// UIHandler handles WebSocket connections from the UI.
type UIHandler struct {
	Hub *Hub
}

// HandleUIWS handles GET /api/v1/ws/ui.
func (h *UIHandler) HandleUIWS(c echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Register with UIHub for broadcast support. wc wraps conn with a
	// per-connection write lock; the ping ticker below writes through it so it
	// never races a concurrent Broadcast on the same conn.
	var wc *wsConn
	if h.Hub != nil && h.Hub.UI != nil {
		var uiID string
		uiID, wc = h.Hub.UI.Register(conn)
		defer h.Hub.UI.Unregister(uiID)
	} else {
		wc = newConn(conn)
	}

	// Keep connection alive with ping/pong.
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return nil
		case <-ticker.C:
			if err := wc.writeControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
				return nil
			}
		}
	}
}
