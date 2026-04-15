package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// ContextKeyNodeID is the key used to store the authenticated node ID in the echo context.
const ContextKeyNodeID = "nodeID"

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
			if token == "" || token != password {
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

// DownloadMiddleware accepts either the admin password or a valid node API key.
// Checks Authorization header and ?token= query param. Used for artifact downloads
// so both the UI (admin) and agents (node key) can access them.
func DownloadMiddleware(password string, nodeStore store.NodeStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := extractBearer(c.Request().Header.Get("Authorization"))
			if token == "" {
				token = c.QueryParam("token")
			}
			if token == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			// Check admin password
			if token == password {
				return next(c)
			}
			// Check node API key
			node, err := nodeStore.GetByAPIKey(c.Request().Context(), token)
			if err == nil && node != nil {
				return next(c)
			}
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}
	}
}

// RegistrationTokenAuth returns an Echo middleware that reads the JSON body,
// checks for a "registrationToken" field matching the expected token, and
// resets the body so downstream handlers can read it again.
func RegistrationTokenAuth(token string) echo.MiddlewareFunc {
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

			if body.RegistrationToken != token {
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
