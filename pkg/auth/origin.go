package auth

import (
	"net/http"
	"net/url"
	"strings"
)

// OriginAllowed reports whether a request's Origin header names this server.
// Browsers send Origin on WebSocket upgrades and on cross-origin and
// state-changing requests; agents and the SDK do not, so a request without one
// is allowed. With one, its host and port must equal the request's Host, or
// the host of publicURL for a reverse proxy that rewrites Host.
func OriginAllowed(r *http.Request, publicURL string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	if publicURL == "" {
		return false
	}
	pub, err := url.Parse(publicURL)
	if err != nil || pub.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, pub.Host)
}
