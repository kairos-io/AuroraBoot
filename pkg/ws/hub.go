package ws

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// Hub tracks active WebSocket connections per node.
type Hub struct {
	connections map[string]*websocket.Conn
	mu          sync.RWMutex
	UI          *UIHub
}

// NewHub creates a new Hub with an embedded UIHub.
func NewHub() *Hub {
	return &Hub{
		connections: make(map[string]*websocket.Conn),
		UI:          NewUIHub(),
	}
}

// Register stores the WebSocket connection for the given node ID.
func (h *Hub) Register(nodeID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[nodeID] = conn
}

// Unregister removes the WebSocket connection for the given node ID.
func (h *Hub) Unregister(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, nodeID)
}

// SendCommand sends a command message to the specified node via WebSocket.
// Returns an error if the node is not connected.
func (h *Hub) SendCommand(nodeID string, cmd any) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, ok := h.connections[nodeID]

	if !ok || conn == nil {
		return fmt.Errorf("node %s is not connected", nodeID)
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal command data: %w", err)
	}

	msg := wsMessage{
		Type: "command",
		Data: data,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal ws message: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

// UIHub tracks active WebSocket connections from UI clients.
type UIHub struct {
	connections map[string]*websocket.Conn
	mu          sync.RWMutex
	counter     atomic.Int64
}

// NewUIHub creates a new UIHub.
func NewUIHub() *UIHub {
	return &UIHub{connections: make(map[string]*websocket.Conn)}
}

// Register stores the WebSocket connection and returns its unique ID.
func (h *UIHub) Register(conn *websocket.Conn) string {
	id := fmt.Sprintf("ui-%d", h.counter.Add(1))
	h.mu.Lock()
	h.connections[id] = conn
	h.mu.Unlock()
	return id
}

// Unregister removes the WebSocket connection with the given ID.
func (h *UIHub) Unregister(id string) {
	h.mu.Lock()
	delete(h.connections, id)
	h.mu.Unlock()
}

// Broadcast sends a message to all connected UI clients.
func (h *UIHub) Broadcast(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conn := range h.connections {
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}
}

// BroadcastLogChunk fans out a chunk of build log to every connected UI
// client as a {"type":"build-log","data":{"id":…,"chunk":…}} envelope.
// Clients filter by build ID on the receive side; broadcasting to all is
// cheap and matches the single-tenant deployment model of daedalus.
func (h *UIHub) BroadcastLogChunk(buildID string, chunk string) {
	h.Broadcast(map[string]any{
		"type": "build-log",
		"data": map[string]any{
			"id":    buildID,
			"chunk": chunk,
		},
	})
}

// IsOnline returns true if the node has an active WebSocket connection.
func (h *Hub) IsOnline(nodeID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.connections[nodeID]
	return ok
}

// OnlineCount returns the number of currently connected nodes.
func (h *Hub) OnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}
