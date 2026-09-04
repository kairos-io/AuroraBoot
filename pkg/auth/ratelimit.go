package auth

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

// Default rate limits for the node-driven fleet endpoints (fleet-server
// hardening, kairos-io/kairos#4117). They are deliberately generous: a healthy
// node heartbeats on the order of once every few tens of seconds, so these caps
// only ever bite a runaway or a flood, never normal operation. Admin-
// authenticated traffic (e.g. the CAPI infra provider) is never limited — see
// NodeRateLimiter.
const (
	// DefaultNodeRateLimitRPS / DefaultNodeRateLimitBurst cap per-node heartbeat
	// and command polling.
	DefaultNodeRateLimitRPS   = 5.0
	DefaultNodeRateLimitBurst = 20

	// DefaultRegisterRateLimitRPS / DefaultRegisterRateLimitBurst cap registration
	// attempts per client IP, so a leaked or guessed registration token cannot be
	// used to flood the fleet with fake nodes (or to brute-force the token).
	// ~30 sustained attempts/min with a burst of 20.
	DefaultRegisterRateLimitRPS   = 0.5
	DefaultRegisterRateLimitBurst = 20

	// rateLimitExpiry is how long an idle per-key bucket is kept before the memory
	// store evicts it. Comfortably longer than any real client's request cadence.
	rateLimitExpiry = 5 * time.Minute
)

// NodeRateLimiter returns middleware that rate-limits node-driven requests keyed
// on the authenticated node ID, while never limiting admin-authenticated
// requests. It must run AFTER the node-auth middleware (NodeAPIKeyMiddleware or
// AgentOrAdminMiddleware) so the node ID is already in the context.
//
// Admin requests set no node ID (AuthNodeID == ""), so the Skipper lets them
// straight through: this is what keeps the CAPI infra provider's admin-bearer
// polling of the shared command routes exempt. On the heartbeat route, where only
// a node ever authenticates, the node ID is always present and every request is
// counted against that node's bucket.
//
// A non-positive rps disables limiting (the middleware passes through), so the
// server can turn the limiter off without special-casing the wiring.
func NodeRateLimiter(rps float64, burst int) echo.MiddlewareFunc {
	if rps <= 0 {
		return passthrough
	}
	if burst <= 0 {
		burst = DefaultNodeRateLimitBurst
	}
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: newRateLimiterStore(rps, burst),
		Skipper: func(c echo.Context) bool {
			// Admin-authenticated requests carry no node ID; never limit them.
			return AuthNodeID(c) == ""
		},
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return AuthNodeID(c), nil
		},
		DenyHandler: rateLimitDenyHandler,
	})
}

// RegistrationRateLimiter returns middleware that rate-limits registration
// attempts keyed on the client IP, regardless of whether the registration token
// is valid. Register it BEFORE RegistrationTokenAuth so invalid-token attempts
// are throttled too — otherwise the token check would run (and cost work) on
// every brute-force attempt before the limiter ever saw it.
//
// The key is echo's RealIP, which honours X-Forwarded-For / X-Real-IP. Behind a
// trusted proxy that overwrites those headers this is the real client IP; a
// directly-exposed server with no such proxy sees the TCP peer. Because a client
// that can set its own X-Forwarded-For could rotate the key to evade this limit,
// the limiter is a flood speed-bump, not the access control — the registration
// token is. Operators who need a hard per-client boundary should front the server
// with a proxy that rewrites the forwarding headers.
//
// A non-positive rps disables limiting (the middleware passes through).
func RegistrationRateLimiter(rps float64, burst int) echo.MiddlewareFunc {
	if rps <= 0 {
		return passthrough
	}
	if burst <= 0 {
		burst = DefaultRegisterRateLimitBurst
	}
	return middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: newRateLimiterStore(rps, burst),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		DenyHandler: rateLimitDenyHandler,
	})
}

// newRateLimiterStore builds an in-memory token-bucket store with the given
// sustained rate and burst, evicting idle buckets after rateLimitExpiry.
func newRateLimiterStore(rps float64, burst int) middleware.RateLimiterStore {
	return middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
		Rate:      rate.Limit(rps),
		Burst:     burst,
		ExpiresIn: rateLimitExpiry,
	})
}

// passthrough is a no-op middleware used when a limiter is disabled.
func passthrough(next echo.HandlerFunc) echo.HandlerFunc { return next }

// rateLimitDenyHandler answers a throttled request with 429 and the same small
// JSON error shape the rest of the API uses.
func rateLimitDenyHandler(c echo.Context, _ string, _ error) error {
	return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
}
