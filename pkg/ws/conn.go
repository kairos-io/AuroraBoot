package ws

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// wsConn wraps a *websocket.Conn with a per-connection write mutex.
//
// gorilla/websocket permits at most one concurrent writer per connection: it is
// the caller's responsibility to serialize writes. In this server several
// goroutines write to the same connection — the hub's SendCommand/Broadcast, the
// agent read-loop's response writes, and the UI ping ticker — so every write
// must go through writeMessage/writeControl to take this lock. The hub's map
// mutex (Hub.mu / UIHub.mu) guards map access only and is intentionally separate
// from this per-connection write lock.
type wsConn struct {
	*websocket.Conn
	writeMu sync.Mutex
}

// newConn wraps a raw gorilla connection so all writes are serialized.
func newConn(c *websocket.Conn) *wsConn {
	return &wsConn{Conn: c}
}

// writeMessage serializes a data-frame write to the connection.
func (c *wsConn) writeMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.Conn.WriteMessage(messageType, data)
}

// writeControl serializes a control-frame write (e.g. ping) to the connection.
func (c *wsConn) writeControl(messageType int, data []byte, deadline time.Time) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.Conn.WriteControl(messageType, data, deadline)
}
