package handlers

import (
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// resetExpired reports whether an in-flight reset has reached its deadline.
// A negative timeout disables expiration; zero is normalized by the handler's
// configuration before this helper is called.
func resetExpired(node *store.ManagedNode, timeout time.Duration, now time.Time) bool {
	if timeout < 0 || node.ResetRequestedAt == nil {
		return false
	}
	if node.ResetState != store.ResetStatePending && node.ResetState != store.ResetStateInProgress {
		return false
	}
	return !now.Before(node.ResetRequestedAt.Add(timeout))
}
