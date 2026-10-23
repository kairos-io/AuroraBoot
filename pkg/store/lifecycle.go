package store

import (
	"context"
	"encoding/json"
	"time"
)

// ResolveReset advances a node's automatic-reset lifecycle
// (kairos-io/kairos#4255) from the boot state it reports after a reset reboot:
//   - autoreset        → in-progress (the node booted the statereset entry and is
//     mid-reset; the following boot will be active)
//   - active           → done (the reset finished and the node is healthy again),
//     stamping LastReset
//   - passive/recovery → failed (the active image did not survive the reset)
//
// The caller has established that the node rebooted. Transitions are
// compare-and-set so concurrent reports resolve exactly once. Best-effort: a
// store error is ignored and the next report retries. A node not awaiting a
// reset is left untouched.
func ResolveReset(ctx context.Context, nodes NodeStore, node *ManagedNode, bootState string) {
	if node.ResetState != ResetStatePending && node.ResetState != ResetStateInProgress {
		return
	}
	inFlight := []string{ResetStatePending, ResetStateInProgress}
	switch bootState {
	case BootStateAutoReset:
		_, _ = nodes.AdvanceReset(ctx, node.ID, []string{ResetStatePending}, ResetStateInProgress, false)
	case BootStateActive:
		_, _ = nodes.AdvanceReset(ctx, node.ID, inFlight, ResetStateDone, true)
	case BootStatePassive, BootStateRecovery:
		_, _ = nodes.AdvanceReset(ctx, node.ID, inFlight, ResetStateFailed, false)
	}
}

// ResolveUpgrade advances a node's upgrade lifecycle from the boot state it
// reports after the upgrade reboot:
//   - active           → done (the node came back up on the upgraded image),
//     stamping LastUpgrade
//   - passive/recovery → failed (the upgraded active image did not boot and the
//     node fell back)
//
// Any other boot state leaves the upgrade pending. The caller has established
// that the node rebooted. Transitions are compare-and-set so concurrent reports
// resolve exactly once. Best-effort: a store error is ignored and the next
// report retries. A node not awaiting an upgrade is left untouched.
func ResolveUpgrade(ctx context.Context, nodes NodeStore, node *ManagedNode, bootState string) {
	if node.UpgradeState != UpgradeStatePending {
		return
	}
	inFlight := []string{UpgradeStatePending}
	switch bootState {
	case BootStateActive:
		_, _ = nodes.AdvanceUpgrade(ctx, node.ID, inFlight, UpgradeStateDone, true)
	case BootStatePassive, BootStateRecovery:
		_, _ = nodes.AdvanceUpgrade(ctx, node.ID, inFlight, UpgradeStateFailed, false)
	}
}

// SettleFromHeartbeat resolves a node's pending upgrade or reset from a
// heartbeat. An agent that already holds its credentials does not register
// again after a reboot, so its heartbeats are the only report of how the
// reboot went.
//
// A heartbeat proves a reboot only when the node booted after the command that
// owns the pending state completed: the agent completes an upgrade or reset
// once it has scheduled the reboot, and a heartbeat from the boot before that
// still carries the old boot state. bootedAt comes from the uptime the
// heartbeat reports; the caller skips heartbeats that carry none.
//
// Pass the boot state the heartbeat reported; an empty one settles nothing.
// Best-effort, like the resolvers it calls.
func SettleFromHeartbeat(ctx context.Context, nodes NodeStore, commands CommandStore, nodeID, bootState string, bootedAt time.Time) {
	if bootState == "" {
		return
	}
	node, err := nodes.GetByID(ctx, nodeID)
	if err != nil || node == nil {
		return
	}
	upgradePending := node.UpgradeState == UpgradeStatePending
	resetPending := node.ResetState == ResetStatePending || node.ResetState == ResetStateInProgress
	if !upgradePending && !resetPending {
		return
	}
	cmds, err := commands.ListByNode(ctx, nodeID)
	if err != nil {
		return
	}
	if upgradePending && rebootedSince(cmds, CmdUpgrade, node.UpgradeRequestedAt, bootedAt) {
		ResolveUpgrade(ctx, nodes, node, bootState)
	}
	if resetPending && rebootedSince(cmds, CmdReset, node.ResetRequestedAt, bootedAt) {
		ResolveReset(ctx, nodes, node, bootState)
	}
}

// rebootedSince reports whether bootedAt is after the completion of the
// command of the given kind that owns the node's pending state: the latest
// Completed one that finished at or after requestedAt. A Completed command that
// finished before the request belongs to an earlier operation.
func rebootedSince(cmds []*NodeCommand, command string, requestedAt *time.Time, bootedAt time.Time) bool {
	var owner *time.Time
	for _, cmd := range cmds {
		if cmd.Command != command || cmd.Phase != CommandCompleted || cmd.CompletedAt == nil {
			continue
		}
		if requestedAt != nil && cmd.CompletedAt.Before(*requestedAt) {
			continue
		}
		if owner == nil || cmd.CompletedAt.After(*owner) {
			owner = cmd.CompletedAt
		}
	}
	return owner != nil && bootedAt.After(*owner)
}

// BootedAt returns when a node booted, from the uptime in a heartbeat's metrics
// sample, and false when the sample carries no uptime. An uptime of 0 is a node
// that has just booted, so presence is checked rather than the value.
func BootedAt(now time.Time, metrics json.RawMessage) (time.Time, bool) {
	if len(metrics) == 0 {
		return time.Time{}, false
	}
	var m struct {
		UptimeSeconds *uint64 `json:"uptimeSeconds"`
	}
	if err := json.Unmarshal(metrics, &m); err != nil || m.UptimeSeconds == nil {
		return time.Time{}, false
	}
	return now.Add(-time.Duration(*m.UptimeSeconds) * time.Second), true
}
