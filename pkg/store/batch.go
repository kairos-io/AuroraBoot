package store

import (
	"context"
	"fmt"
)

// BatchOutcome is the state of one fan-out, derived from the commands it
// created rather than stored, so it cannot drift from them.
type BatchOutcome struct {
	BatchID  string `json:"batchID"`
	Command  string `json:"command"`
	FailFast bool   `json:"failFast"`
	// MaxConcurrency is the in-flight limit the batch was created with, zero
	// when it was not throttled.
	MaxConcurrency int `json:"maxConcurrency,omitempty"`
	// Phase is the batch as a whole: Pending while any node still has work
	// ahead of it, then Failed if any node failed, Canceled if the batch was
	// stopped before every node ran, and Completed when every node succeeded.
	Phase string `json:"phase"`
	Total int    `json:"total"`
	// Queued is how many nodes are held back by the concurrency limit and
	// have not been offered their command yet. They are counted in Pending as
	// well, so Pending stays "nodes with work still ahead of them".
	Queued    int `json:"queued,omitempty"`
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Canceled  int `json:"canceled"`
	Expired   int `json:"expired"`
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
		if cmd.MaxConcurrency > out.MaxConcurrency {
			out.MaxConcurrency = cmd.MaxConcurrency
		}
		if cmd.Phase == CommandQueued {
			out.Queued++
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

// ReleaseBatchSlot hands the slot a finished command held to the next node
// waiting in its throttled batch, and returns the commands that became
// deliverable so the caller can push them.
//
// It is a free function over the CommandStore for the same reason
// CancelBatchAfterFailure is: every path that can move a command to a terminal
// phase has to release its slot, and pkg/ws cannot import pkg/handlers. A
// batch with no concurrency limit, a command outside a batch, and any
// non-terminal phase all release nothing.
//
// The caller has to push what comes back. A promoted command is Pending, and
// the agent neither polls for commands nor is told about one it was never
// offered, so without the push the rollout stops at the limit.
func ReleaseBatchSlot(ctx context.Context, cs CommandStore, cmd *NodeCommand) ([]*NodeCommand, error) {
	if cs == nil || cmd == nil || cmd.BatchID == "" || cmd.MaxConcurrency <= 0 ||
		!IsTerminalCommandPhase(cmd.Phase) {
		return nil, nil
	}
	return cs.PromoteQueuedInBatch(ctx, cmd.BatchID, cmd.MaxConcurrency)
}
