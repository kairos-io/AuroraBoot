package handlers

import (
	"testing"
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/store"
)

func TestResetExpired(t *testing.T) {
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	old := now.Add(-31 * time.Minute)
	recent := now.Add(-29 * time.Minute)

	tests := []struct {
		name    string
		node    *store.ManagedNode
		timeout time.Duration
		want    bool
	}{
		{name: "overdue pending", node: &store.ManagedNode{ResetState: store.ResetStatePending, ResetRequestedAt: &old}, timeout: 30 * time.Minute, want: true},
		{name: "overdue in progress", node: &store.ManagedNode{ResetState: store.ResetStateInProgress, ResetRequestedAt: &old}, timeout: 30 * time.Minute, want: true},
		{name: "recent pending", node: &store.ManagedNode{ResetState: store.ResetStatePending, ResetRequestedAt: &recent}, timeout: 30 * time.Minute, want: false},
		{name: "missing request time", node: &store.ManagedNode{ResetState: store.ResetStatePending}, timeout: 30 * time.Minute, want: false},
		{name: "terminal state", node: &store.ManagedNode{ResetState: store.ResetStateDone, ResetRequestedAt: &old}, timeout: 30 * time.Minute, want: false},
		{name: "disabled", node: &store.ManagedNode{ResetState: store.ResetStatePending, ResetRequestedAt: &old}, timeout: -1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resetExpired(tt.node, tt.timeout, now); got != tt.want {
				t.Fatalf("resetExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}
