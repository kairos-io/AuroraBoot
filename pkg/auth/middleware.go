package auth

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// ContextKeyNodeID is the key used to store the authenticated node ID in the echo context.
const ContextKeyNodeID = "nodeID"

// secureCompare reports whether a and b are equal using a constant-time
// comparison, so a bearer-token / admin-password check does not leak, via
// response timing, how many leading bytes of the secret matched. Plain string
// == returns early on the first differing byte; subtle.ConstantTimeCompare does
// not, and returns 0 for unequal-length inputs without a data-dependent branch.
func secureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// AuthNodeID returns the authenticated node ID set by NodeAPIKeyMiddleware, or
// "" when the request was not authenticated as a node (e.g. an admin-bearer
// request). Handlers shared between the agent and admin groups use the empty
// return to distinguish the two callers.
func AuthNodeID(c echo.Context) string {
	id, _ := c.Get(ContextKeyNodeID).(string)
	return id
}

// RequireNodeMatch enforces that the authenticated node (set by
// NodeAPIKeyMiddleware) matches the nodeID path parameter. It is meant to wrap
// agent-group routes carrying a :nodeID segment so a node can only act on its
// own resources — preventing one registered node from impersonating another
// (BOLA). Returns 403 on mismatch and 401 if no node identity is present (the
// middleware should always run first, so the latter is defence in depth).
func RequireNodeMatch(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		authID := AuthNodeID(c)
		if authID == "" {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}
		if c.Param("nodeID") != authID {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "forbidden"})
		}
		return next(c)
	}
}

// AdminMiddleware returns an Echo middleware that checks the Authorization header
// for a Bearer token matching the given admin password.
func AdminMiddleware(password string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Check Authorization header first, fall back to ?token= query param (for download links)
			token := extractBearer(c.Request().Header.Get("Authorization"))
			if token == "" {
				token = c.QueryParam("token")
			}
			if token == "" || !secureCompare(token, password) {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			return next(c)
		}
	}
}

// NodeAPIKeyMiddleware returns an Echo middleware that checks the Authorization header
// for a Bearer token matching a node's API key. On success it sets the node ID in the context.
func NodeAPIKeyMiddleware(nodeStore store.NodeStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := extractBearer(c.Request().Header.Get("Authorization"))
			if token == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			node, err := nodeStore.GetByAPIKey(c.Request().Context(), token)
			if err != nil || node == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			c.Set(ContextKeyNodeID, node.ID)
			return next(c)
		}
	}
}

// AgentOrAdminMiddleware authenticates a request as EITHER an admin (bearer
// token == admin password) OR a node (bearer token == a node API key). It is
// used for the two routes that are legitimately shared between the agent and
// the admin/UI: GET /nodes/:nodeID/commands and PUT
// /nodes/:nodeID/commands/:commandID/status.
//
// On a node match it sets ContextKeyNodeID so downstream handlers can tell the
// caller is a node and enforce node-scoping (RequireNodeMatch / node-scoped
// store updates). On an admin match it sets nothing, leaving AuthNodeID empty,
// so admins act across nodes. Admin is checked first: the admin password is not
// a valid node API key, so the order is unambiguous.
//
// This middleware exists to avoid registering the same path twice (once per
// auth group), which Echo resolves by silently shadowing the first
// registration — previously routing agent command polls into the admin-only
// handler and 401-ing every agent.
func AgentOrAdminMiddleware(password string, nodeStore store.NodeStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := extractBearer(c.Request().Header.Get("Authorization"))
			if token == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			// Admin first — the admin password is never a valid node API key,
			// so the order is unambiguous.
			if secureCompare(token, password) {
				return next(c)
			}
			node, err := nodeStore.GetByAPIKey(c.Request().Context(), token)
			if err != nil || node == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			c.Set(ContextKeyNodeID, node.ID)
			return next(c)
		}
	}
}

// ArtifactImageMiddleware authorizes GET /api/v1/artifacts/:id/image for either an
// admin or a node that has been assigned this artifact by an upgrade command.
//
// The container image is the one build artifact a node legitimately downloads: an
// operator queues an upgrade / upgrade-recovery command whose source is
// "artifact:<id>", the node polls it, and the agent pulls that artifact's image
// with its node API key. So a node is authorized for exactly the artifacts some
// command told it to install — never an arbitrary one. This stops a node (or a
// leaked node API key) from pulling every image the fleet has ever built, which
// may embed a cloud-config and secrets.
//
// Admin access is unchanged and still full, via the Authorization header or the
// ?token= query param (the latter for the UI's browser download links; the CAPI
// infra provider authenticates as admin over the header). A NODE API key, by
// contrast, is accepted ONLY from the Authorization header, never from ?token= —
// a node credential in a URL would leak through access logs, proxies, browser
// history and Referer headers, and no node flow supplies its key that way.
func ArtifactImageMiddleware(password string, nodeStore store.NodeStore, commandStore store.CommandStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			headerToken := extractBearer(c.Request().Header.Get("Authorization"))
			queryToken := c.QueryParam("token")
			if headerToken == "" && queryToken == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			// Admin: full access, from either the header or ?token=.
			if secureCompare(headerToken, password) || secureCompare(queryToken, password) {
				return next(c)
			}
			// Otherwise the caller must be a node — authenticated ONLY via the
			// Authorization header — and only for an artifact a command assigned it.
			if headerToken == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			node, err := nodeStore.GetByAPIKey(c.Request().Context(), headerToken)
			if err != nil || node == nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			if !nodeAssignedArtifact(c.Request().Context(), commandStore, node.ID, c.Param("id")) {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "forbidden"})
			}
			c.Set(ContextKeyNodeID, node.ID)
			return next(c)
		}
	}
}

// nodeAssignedArtifact reports whether some upgrade / upgrade-recovery command
// queued for nodeID names this artifact as its source ("artifact:<id>"). It does
// not filter on command phase: once an operator has told a node to install an
// artifact, re-fetching that same image (a retry, a resumed upgrade) is not a new
// exposure — the boundary being enforced is WHICH artifact, not how many times. A
// store error fails closed (treated as not assigned).
func nodeAssignedArtifact(ctx context.Context, commandStore store.CommandStore, nodeID, artifactID string) bool {
	if commandStore == nil || nodeID == "" || artifactID == "" {
		return false
	}
	cmds, err := commandStore.ListByNode(ctx, nodeID)
	if err != nil {
		return false
	}
	want := "artifact:" + artifactID
	for _, cmd := range cmds {
		if cmd == nil {
			continue
		}
		if (cmd.Command == store.CmdUpgrade || cmd.Command == store.CmdUpgradeRecovery) && cmd.Args["source"] == want {
			return true
		}
	}
	return false
}

// RegistrationTokenAuth returns an Echo middleware that reads the JSON body,
// checks for a "registrationToken" field matching the expected token, and
// resets the body so downstream handlers can read it again.
//
// The token is supplied as a pointer because SettingsHandler.RotateRegistrationToken
// updates it in place; the middleware must read the current value on every
// request, otherwise rotated tokens wouldn't actually invalidate old ones
// (a pre-rotation registration would keep succeeding against the middleware's
// captured copy).
func RegistrationTokenAuth(token *string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			bodyBytes, err := io.ReadAll(c.Request().Body)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "cannot read body"})
			}

			var body struct {
				RegistrationToken string `json:"registrationToken"`
			}
			if err := json.Unmarshal(bodyBytes, &body); err != nil {
				c.Request().Body = io.NopCloser(bytes.NewReader(bodyBytes))
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
			}

			// Reset body for downstream handler
			c.Request().Body = io.NopCloser(bytes.NewReader(bodyBytes))

			current := ""
			if token != nil {
				current = *token
			}
			if body.RegistrationToken != current {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid registration token"})
			}
			return next(c)
		}
	}
}

func extractBearer(header string) string {
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimPrefix(header, "Bearer ")
	}
	return ""
}
