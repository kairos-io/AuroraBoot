package ws

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// Hub tracks active WebSocket connections per node.
//
// mu guards the connections map only. Each stored *wsConn carries its own write
// mutex, so concurrent SendCommand calls to the same node — and any other writer
// to that connection — are serialized at the connection level, as gorilla
// requires, without holding the map lock across a network write.
type Hub struct {
	connections map[string]*wsConn
	mu          sync.RWMutex
	UI          *UIHub
}

// NewHub creates a new Hub with an embedded UIHub.
func NewHub() *Hub {
	return &Hub{
		connections: make(map[string]*wsConn),
		UI:          NewUIHub(),
	}
}

// Register stores the WebSocket connection for the given node ID and returns the
// wrapped connection whose writeMessage/writeControl methods serialize writes.
// Callers (e.g. the agent read-loop) must route all of their own writes through
// the returned wrapper so they don't race the hub's writes on the same conn.
func (h *Hub) Register(nodeID string, conn *websocket.Conn) *wsConn {
	wc := newConn(conn)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connections[nodeID] = wc
	return wc
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
	// Hold the map lock only long enough to look up the connection; the actual
	// network write happens under the per-connection write lock so concurrent
	// SendCommand calls to the same node are serialized without blocking the map.
	h.mu.RLock()
	conn, ok := h.connections[nodeID]
	h.mu.RUnlock()

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

	if err := conn.writeMessage(websocket.TextMessage, msgBytes); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	return nil
}

// UIHub tracks active WebSocket connections from UI clients.
//
// As with Hub, mu guards the connections map only; each stored *wsConn carries
// its own write mutex so concurrent Broadcasts (e.g. a build-log chunk and a
// deploy-progress update) never race on the same UI connection.
type UIHub struct {
	connections map[string]*wsConn
	mu          sync.RWMutex
	counter     atomic.Int64
}

// NewUIHub creates a new UIHub.
func NewUIHub() *UIHub {
	return &UIHub{connections: make(map[string]*wsConn)}
}

// Register stores the WebSocket connection and returns its unique ID and the
// wrapped connection. The UI handler must route its own writes (the ping ticker)
// through the returned wrapper so they don't race the hub's Broadcasts.
func (h *UIHub) Register(conn *websocket.Conn) (string, *wsConn) {
	id := fmt.Sprintf("ui-%d", h.counter.Add(1))
	wc := newConn(conn)
	h.mu.Lock()
	h.connections[id] = wc
	h.mu.Unlock()
	return id, wc
}

// Unregister removes the WebSocket connection with the given ID.
func (h *UIHub) Unregister(id string) {
	h.mu.Lock()
	delete(h.connections, id)
	h.mu.Unlock()
}

// Count returns the number of currently connected UI clients.
func (h *UIHub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}

// Broadcast sends a message to all connected UI clients.
func (h *UIHub) Broadcast(msg any) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	// Snapshot the connection set under the map lock, then write outside it so a
	// slow client can't block other Broadcasts. Each write takes the per-conn
	// write lock, serializing concurrent Broadcasts on the same connection.
	h.mu.RLock()
	conns := make([]*wsConn, 0, len(h.connections))
	for _, conn := range h.connections {
		conns = append(conns, conn)
	}
	h.mu.RUnlock()
	for _, conn := range conns {
		_ = conn.writeMessage(websocket.TextMessage, data)
	}
}

// BroadcastLogChunk fans out a chunk of build log to every connected UI
// client as a {"type":"build-log","data":{"id":…,"chunk":…}} envelope.
// Clients filter by build ID on the receive side; broadcasting to all is
// cheap and matches the single-tenant deployment model of auroraboot.
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
