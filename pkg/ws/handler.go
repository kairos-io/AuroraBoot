package ws

import (
	"context"
	"encoding/json"
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

// AgentHandler handles WebSocket connections from agents.
type AgentHandler struct {
	Hub      *Hub
	Nodes    store.NodeStore
	Commands store.CommandStore
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

	// Register in hub and mark node online.
	h.Hub.Register(node.ID, conn)
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
	h.sendPendingCommands(node.ID, conn)

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
			h.handleCommandStatus(msg.Data)
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
	if err := h.Nodes.UpdateHeartbeat(ctx, nodeID, hb.AgentVersion, hb.OSRelease); err != nil {
		log.Printf("ws: failed to update heartbeat for node %s: %v", nodeID, err)
	}
	if err := h.Nodes.UpdatePhase(ctx, nodeID, store.PhaseOnline); err != nil {
		log.Printf("ws: failed to update node phase: %v", err)
	}
}

func (h *AgentHandler) handleCommandStatus(data json.RawMessage) {
	var status commandStatusData
	if err := json.Unmarshal(data, &status); err != nil {
		log.Printf("ws invalid command_status: %v", err)
		return
	}

	// context.Background() is correct here: handleCommandStatus is called from
	// the WS read loop which outlives the original HTTP request context.
	ctx := context.Background()
	if err := h.Commands.UpdateStatus(ctx, status.ID, status.Phase, status.Result); err != nil {
		log.Printf("ws: failed to update command status for %s: %v", status.ID, err)
	}

	if h.Hub != nil && h.Hub.UI != nil {
		h.Hub.UI.Broadcast(wsMessage{
			Type: "command_update",
			Data: data,
		})
	}
}

func (h *AgentHandler) sendPendingCommands(nodeID string, conn *websocket.Conn) {
	ctx := context.Background()
	cmds, err := h.Commands.GetPending(ctx, nodeID)
	if err != nil || len(cmds) == 0 {
		return
	}

	var delivered []string
	for _, cmd := range cmds {
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

		if err := conn.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
			log.Printf("ws: failed to send pending command %s to node %s: %v", cmd.ID, nodeID, err)
			break
		}
		delivered = append(delivered, cmd.ID)
	}

	if len(delivered) > 0 {
		if err := h.Commands.MarkDelivered(ctx, delivered); err != nil {
			log.Printf("ws: failed to mark commands as delivered: %v", err)
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

	// Register with UIHub for broadcast support.
	var uiID string
	if h.Hub != nil && h.Hub.UI != nil {
		uiID = h.Hub.UI.Register(conn)
		defer h.Hub.UI.Unregister(uiID)
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
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return nil
			}
		}
	}
}
