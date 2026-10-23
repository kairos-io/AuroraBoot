package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Batch phases.
const (
	BatchRunning   = "running"
	BatchCompleted = "completed"
	BatchFailed    = "failed"
	BatchStopped   = "stopped"
)

// Batch node states. A node moves waiting, running, then rebooting when its
// command reboots the machine, and ends in one of the final states.
const (
	BatchNodeWaiting   = "waiting"
	BatchNodeRunning   = "running"
	BatchNodeRebooting = "rebooting"
	BatchNodeDone      = "done"
	BatchNodeFailed    = "failed"
	BatchNodeTimedOut  = "timed_out"
	BatchNodeSkipped   = "skipped"
	BatchNodeCanceled  = "canceled"
)

// DefaultRebootTimeoutSeconds is how long a rebooting node may take to come
// back when the batch does not say.
const DefaultRebootTimeoutSeconds = 600

// IsFinalBatchNodeState reports whether a node in state takes no further part
// in its batch.
func IsFinalBatchNodeState(state string) bool {
	switch state {
	case BatchNodeDone, BatchNodeFailed, BatchNodeTimedOut, BatchNodeSkipped, BatchNodeCanceled:
		return true
	}
	return false
}

// RebootsNode reports whether AuroraBoot reboots the node for command: upgrade, reset, reboot.
func RebootsNode(command string) bool {
	switch command {
	case CmdUpgrade, CmdReset, CmdReboot:
		return true
	}
	return false
}

// CommandBatch is a rolling run of one command over a set of nodes.
type CommandBatch struct {
	ID                   string            `json:"id" gorm:"primaryKey"`
	Command              string            `json:"command"`
	Args                 map[string]string `json:"args" gorm:"serializer:json"`
	Selection            string            `json:"selection"`
	GroupID              *string           `json:"groupID" gorm:"index" extensions:"x-nullable"`
	Concurrency          int               `json:"concurrency"`
	StopOnFailure        bool              `json:"stopOnFailure"`
	RebootTimeoutSeconds int               `json:"rebootTimeoutSeconds"`
	Phase                string            `json:"phase" gorm:"index"`
	CreatedAt            time.Time         `json:"createdAt"`
	FinishedAt           *time.Time        `json:"finishedAt"`
	StoppedAt            *time.Time        `json:"stoppedAt"`
	StopReason           string            `json:"stopReason"`
	StoppedByNodeID      *string           `json:"stoppedByNodeID" extensions:"x-nullable"`
	// Counts maps each node state to how many nodes are in it.
	Counts map[string]int `json:"counts" gorm:"-"`
	// Nodes is filled by BatchGet only.
	Nodes []*BatchNode `json:"nodes,omitempty" gorm:"foreignKey:BatchID"`
}

// BatchNode is one node's place in a batch.
type BatchNode struct {
	BatchID   string     `json:"batchID" gorm:"primaryKey"`
	NodeID    string     `json:"nodeID" gorm:"primaryKey;index"`
	Position  int        `json:"position"`
	Hostname  string     `json:"hostname"`
	CommandID *string    `json:"commandID" gorm:"index" extensions:"x-nullable"`
	State     string     `json:"state" gorm:"index"`
	Message   string     `json:"message"`
	StartedAt *time.Time `json:"startedAt"`
	// RebootingSince is when the reboot the node is waiting on was
	// scheduled: its command's completion, then its autoreset boot.
	RebootingSince *time.Time `json:"rebootingSince"`
	Deadline       *time.Time `json:"deadline"`
	FinishedAt     *time.Time `json:"finishedAt"`
}

// TableName names the per-node table of a batch.
func (BatchNode) TableName() string { return "command_batch_nodes" }

// BatchFilter narrows BatchStore.List.
type BatchFilter struct {
	GroupID string
	NodeID  string
	Limit   int // 0 means 50
	Offset  int
}

var (
	ErrBatchNotFound   = errors.New("batch not found")
	ErrBatchNotRunning = errors.New("batch is not running")
)

// BatchStore persists batches and their per-node rows.
type BatchStore interface {
	// Create writes the batch and its node rows in one transaction. Every row
	// must be waiting; Phase is set to running.
	Create(ctx context.Context, b *CommandBatch, nodes []*BatchNode) error
	// Get returns the batch with Nodes ordered by Position and Counts filled.
	Get(ctx context.Context, id string) (*CommandBatch, error)
	// List returns batches newest first with Counts filled and Nodes empty,
	// and the total number matching the filter.
	List(ctx context.Context, f BatchFilter) ([]*CommandBatch, int, error)
	// BusyNodes returns the IDs among nodeIDs that have a row in a non-final state.
	BusyNodes(ctx context.Context, nodeIDs []string) ([]string, error)
}

// BatchOutcome is the state of one fan-out, derived from the commands it
// created rather than stored, so it cannot drift from them.
type BatchOutcome struct {
	BatchID  string `json:"batchID"`
	Command  string `json:"command"`
	FailFast bool   `json:"failFast"`
	// Phase is the batch as a whole: Pending while any node still has work
	// ahead of it, then Failed if any node failed, Canceled if the batch was
	// stopped before every node ran, and Completed when every node succeeded.
	Phase     string `json:"phase"`
	Total     int    `json:"total"`
	Pending   int    `json:"pending"`
	Running   int    `json:"running"`
	Completed int    `json:"completed"`
	Failed    int    `json:"failed"`
	Canceled  int    `json:"canceled"`
	Expired   int    `json:"expired"`
	// Commands are the per-node rows, kept so a failed batch can still say
	// which node failed and with what result.
	Commands []*NodeCommand `json:"commands"`
}

// SummarizeBatch folds the commands of one batch into its overall outcome.
//
// A node that is still Pending, Delivered or Running is counted as unfinished,
// and an unfinished node keeps the whole batch Pending: an operator watching a
// rollout needs to know it is not over yet even though one node has already
// reported. Only once every node is terminal does the batch take the phase of
// the worst thing that happened to it, because that is the outcome the operator
// has to act on.
func SummarizeBatch(batchID string, cmds []*NodeCommand) BatchOutcome {
	out := BatchOutcome{BatchID: batchID, Total: len(cmds), Commands: cmds}
	for _, cmd := range cmds {
		if out.Command == "" {
			out.Command = cmd.Command
		}
		if cmd.FailFast {
			out.FailFast = true
		}
		switch cmd.Phase {
		case CommandCompleted:
			out.Completed++
		case CommandFailed:
			out.Failed++
		case CommandCanceled:
			out.Canceled++
		case CommandExpired:
			out.Expired++
		case CommandRunning, CommandDelivered:
			out.Running++
		default:
			out.Pending++
		}
	}

	switch {
	case out.Pending > 0 || out.Running > 0:
		out.Phase = CommandPending
	case out.Failed > 0:
		out.Phase = CommandFailed
	case out.Canceled > 0 || out.Expired > 0:
		out.Phase = CommandCanceled
	default:
		out.Phase = CommandCompleted
	}
	return out
}

// CancelBatchAfterFailure stops the rest of a fail-fast batch once one of its
// commands has failed, and reports how many pending commands it canceled.
//
// It is a free function over the CommandStore rather than a method on a handler
// because all three paths that can move a command to Failed have to apply the
// same rule: the agent's REST status report, an admin's status write, and the
// WebSocket command_status frame. pkg/ws cannot import pkg/handlers, so the
// shared rule lives here, next to the phases it reads.
//
// It does nothing for a command outside a batch, for a batch that did not ask
// for fail-fast, and for any phase other than Failed. Cancelling only the
// still-Pending siblings is not a simplification: pushCommand claims a command
// Pending -> Delivered before it writes to the socket and there is no abort
// message in the protocol, so a delivered command is already the node's and has
// to be left to report its own result.
func CancelBatchAfterFailure(ctx context.Context, cs CommandStore, cmd *NodeCommand) (int, error) {
	if cs == nil || cmd == nil || cmd.BatchID == "" || !cmd.FailFast || cmd.Phase != CommandFailed {
		return 0, nil
	}
	reason := fmt.Sprintf("canceled: batch %s stopped after node %s failed", cmd.BatchID, cmd.ManagedNodeID)
	return cs.CancelPendingInBatch(ctx, cmd.BatchID, reason)
}
