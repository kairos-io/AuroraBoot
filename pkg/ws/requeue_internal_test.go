package ws

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// queueStore is the slice of store.CommandStore that sendPendingCommands
// touches: the pending query and the two phase transitions around a delivery.
// The rest of the interface is unreachable from this path and panics so a
// future call site cannot pass unnoticed.
type queueStore struct {
	mu   sync.Mutex
	cmds []*store.NodeCommand
	// afterClaim runs once a claim has landed, which is where a racing status
	// report from the node lands in real life.
	afterClaim func(*store.NodeCommand)
	// releaseErr makes the release fail, the one case requeue can only report.
	releaseErr error
}

func (q *queueStore) GetPending(_ context.Context, nodeID string) ([]*store.NodeCommand, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []*store.NodeCommand
	for _, c := range q.cmds {
		if c.ManagedNodeID == nodeID && c.Phase == store.CommandPending {
			out = append(out, c)
		}
	}
	return out, nil
}

func (q *queueStore) ClaimForDelivery(_ context.Context, id string) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, c := range q.cmds {
		if c.ID == id && c.Phase == store.CommandPending {
			now := time.Now()
			c.Phase = store.CommandDelivered
			c.DeliveredAt = &now
			if q.afterClaim != nil {
				q.afterClaim(c)
			}
			return true, nil
		}
	}
	return false, nil
}

// ReleaseClaim mirrors the store's conditional UPDATE: only a Delivered
// command moves back, so a command the node already started is never pulled
// into the queue behind its back.
func (q *queueStore) ReleaseClaim(_ context.Context, id string) (bool, error) {
	if q.releaseErr != nil {
		return false, q.releaseErr
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, c := range q.cmds {
		if c.ID == id && c.Phase == store.CommandDelivered {
			c.Phase = store.CommandPending
			c.DeliveredAt = nil
			return true, nil
		}
	}
	return false, nil
}

func (q *queueStore) Create(context.Context, *store.NodeCommand) error { panic("not reached") }
func (q *queueStore) GetByID(context.Context, string) (*store.NodeCommand, error) {
	panic("not reached")
}
func (q *queueStore) MarkDelivered(context.Context, []string) error { panic("not reached") }
func (q *queueStore) UpdateStatus(context.Context, string, string, string) error {
	panic("not reached")
}
func (q *queueStore) UpdateStatusForNode(context.Context, string, string, string, string) error {
	panic("not reached")
}
func (q *queueStore) ListByNode(context.Context, string) ([]*store.NodeCommand, error) {
	panic("not reached")
}
func (q *queueStore) Delete(context.Context, string) error         { panic("not reached") }
func (q *queueStore) DeleteTerminal(context.Context, string) error { panic("not reached") }

// deadWSConn is a real upgraded connection that has been closed, which is the
// state the replay can observe for real: the agent reconnects, the handler
// starts draining its queue, and the socket dies before the write lands.
// writeMessage then fails with "use of closed network connection".
func deadWSConn(t *testing.T) *wsConn {
	t.Helper()
	serverConns := make(chan *websocket.Conn, 1)
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		serverConns <- c
	}))
	t.Cleanup(srv.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConns:
	case <-time.After(5 * time.Second):
		t.Fatal("the server never saw the upgraded connection")
	}
	if err := serverConn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	conn := newConn(serverConn)
	if err := conn.writeMessage(websocket.TextMessage, []byte("probe")); err == nil {
		t.Fatal("the closed connection still accepted a write")
	}
	return conn
}

// The reconnect replay claims Pending -> Delivered before it writes, and
// Delivered is the end of the line for delivery: GetPending matches Pending
// only and nothing else moves a command out of Delivered. So a write that
// fails after the claim used to strand the command forever, on the very path
// that exists to recover from a dropped agent (kairos-io/kairos#4932).
func TestSendPendingCommandsRequeuesACommandItCouldNotWrite(t *testing.T) {
	q := &queueStore{cmds: []*store.NodeCommand{
		{ID: "cmd-1", ManagedNodeID: "node-1", Command: "upgrade", Phase: store.CommandPending},
	}}
	h := &AgentHandler{Hub: NewHub(), Commands: q}

	h.sendPendingCommands("node-1", deadWSConn(t))

	pending, err := q.GetPending(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("got %d pending commands, want the undelivered one back in the queue", len(pending))
	}
	if pending[0].ID != "cmd-1" {
		t.Errorf("pending command is %q, want cmd-1", pending[0].ID)
	}
	if pending[0].DeliveredAt != nil {
		t.Error("the requeued command still carries a delivery timestamp")
	}
}

// The write failure means the socket is gone, so the rest of the batch must be
// left alone rather than claimed and dropped one by one.
func TestSendPendingCommandsLeavesTheRestOfTheBatchPending(t *testing.T) {
	q := &queueStore{cmds: []*store.NodeCommand{
		{ID: "cmd-1", ManagedNodeID: "node-1", Command: "upgrade", Phase: store.CommandPending},
		{ID: "cmd-2", ManagedNodeID: "node-1", Command: "reset", Phase: store.CommandPending},
		{ID: "cmd-3", ManagedNodeID: "node-2", Command: "upgrade", Phase: store.CommandPending},
	}}
	h := &AgentHandler{Hub: NewHub(), Commands: q}

	h.sendPendingCommands("node-1", deadWSConn(t))

	pending, err := q.GetPending(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("got %d pending commands for node-1, want both back in the queue", len(pending))
	}
	for _, c := range pending {
		if c.DeliveredAt != nil {
			t.Errorf("command %q still carries a delivery timestamp", c.ID)
		}
	}
}

// A command the node already started is not ours to requeue. The release is
// conditional on Delivered, so a status report that lands between the claim
// and the failed write wins and the command is not handed out a second time.
func TestSendPendingCommandsDoesNotRequeueACommandAlreadyRunning(t *testing.T) {
	q := &queueStore{cmds: []*store.NodeCommand{
		{ID: "cmd-1", ManagedNodeID: "node-1", Command: "upgrade", Phase: store.CommandPending},
	}}
	// The node reports Running the moment the claim lands, before the write
	// to the dead socket gets the chance to fail.
	q.afterClaim = func(c *store.NodeCommand) { c.Phase = store.CommandRunning }

	h := &AgentHandler{Hub: NewHub(), Commands: q}
	h.sendPendingCommands("node-1", deadWSConn(t))

	if got := q.cmds[0].Phase; got != store.CommandRunning {
		t.Errorf("phase is %q, want it left at Running", got)
	}
	pending, err := q.GetPending(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("a running command was pulled back into the queue: %d pending", len(pending))
	}
}

// A release that fails is all the replay can do about it: the command stays
// Delivered and this reconnect is over, but the handler must report it rather
// than drop the error and must not take the rest of the batch down with it.
func TestSendPendingCommandsSurvivesAFailedRelease(t *testing.T) {
	q := &queueStore{
		cmds: []*store.NodeCommand{
			{ID: "cmd-1", ManagedNodeID: "node-1", Command: "upgrade", Phase: store.CommandPending},
		},
		releaseErr: errors.New("database is locked"),
	}
	h := &AgentHandler{Hub: NewHub(), Commands: q}

	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	h.sendPendingCommands("node-1", deadWSConn(t))

	if !strings.Contains(logged.String(), "failed to requeue undelivered command cmd-1") {
		t.Errorf("the failed requeue went unreported; log was:\n%s", logged.String())
	}
}
