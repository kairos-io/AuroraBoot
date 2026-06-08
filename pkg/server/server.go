package server

import (
	"context"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/kairos-io/AuroraBoot/docs"
	netbootpkg "github.com/kairos-io/AuroraBoot/internal/netbootmgr"
	"github.com/kairos-io/AuroraBoot/internal/ui"
	"github.com/kairos-io/AuroraBoot/pkg/auth"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/isoserve"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/kairos-io/AuroraBoot/pkg/ws"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Config holds all dependencies needed by the server.
type Config struct {
	NodeStore             store.NodeStore
	CommandStore          store.CommandStore
	GroupStore            store.GroupStore
	ArtifactStore         store.ArtifactStore
	SecureBootKeySetStore store.SecureBootKeySetStore
	NetbootManager        *netbootpkg.Manager
	DeploymentStore       store.DeploymentStore
	BMCTargetStore        store.BMCTargetStore
	Builder               builder.ArtifactBuilder
	AdminPassword         string
	RegToken              string
	RegTokenFile          string // path where reg token is persisted (for rotation)
	AuroraBootURL         string
	ArtifactsDir          string
	KeysDir               string  // base directory for SecureBoot key sets
	Hub                   *ws.Hub // optional, created if nil
	// ISOServe serves a local artifact ISO over a tokenized, BMC-reachable URL
	// for Redfish virtual-media deployments. Optional; when nil the Redfish
	// deploy path requires an explicit imageUrl.
	ISOServe *isoserve.Server
	// BaseContext, when non-nil, is the parent context for background deploy
	// goroutines so a server shutdown cancels in-flight Redfish deploys. Defaults
	// to context.Background().
	BaseContext context.Context
}

// redactToken returns requestURI with the value of any "token" query parameter
// replaced by "REDACTED", so credentials passed as ?token= (WebSocket auth and
// artifact download links) never reach the access log. Other query parameters
// and the path are preserved to keep the log useful. If the URI cannot be
// parsed but still references a token, it is fully redacted to err on the side
// of never emitting the secret.
func redactToken(requestURI string) string {
	if !strings.Contains(requestURI, "token=") {
		return requestURI
	}

	path := requestURI
	rawQuery := ""
	if i := strings.IndexByte(requestURI, '?'); i >= 0 {
		path = requestURI[:i]
		rawQuery = requestURI[i+1:]
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		// Could not parse the query string; refuse to risk leaking the token.
		return path + "?REDACTED"
	}
	if !values.Has("token") {
		return requestURI
	}
	values.Set("token", "REDACTED")
	return path + "?" + values.Encode()
}

// New creates and configures an Echo server with all routes and middleware.
func New(cfg Config) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	// Use a request logger whose URI is run through redactToken before it is
	// written. All three auth schemes accept the credential as a ?token= query
	// param (agent and UI WebSockets, artifact download links), so the raw
	// request URI would otherwise leak admin passwords, node API keys, and the
	// registration token straight into the access log (and any downstream proxy
	// logs). RequestLoggerWithConfig (the non-deprecated logger) lets us emit a
	// redacted URI explicitly instead of the verbatim ${uri} the default
	// middleware.Logger() prints.
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogRemoteIP: true,
		LogMethod:   true,
		LogURI:      true,
		LogStatus:   true,
		LogLatency:  true,
		LogValuesFunc: func(_ echo.Context, v middleware.RequestLoggerValues) error {
			log.Printf("%s %s %s %d %s",
				v.RemoteIP, v.Method, redactToken(v.URI), v.Status, v.Latency)
			return nil
		},
	}))
	e.Use(middleware.Recover())

	regToken := cfg.RegToken

	// Create WebSocket hub.
	hub := cfg.Hub
	if hub == nil {
		hub = ws.NewHub()
	}

	// Health endpoints
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/readyz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// OpenAPI spec + Swagger UI. The spec is generated from the swag
	// annotations on the handlers via `make openapi`; the generated
	// docs package embeds the swagger.json and swagger.yaml strings.
	// Both endpoints are public — they're just the schema, no secrets.
	e.GET("/api/v1/openapi.json", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "application/json", []byte(docs.SwaggerInfo.ReadDoc()))
	})
	e.GET("/api/v1/openapi.yaml", func(c echo.Context) error {
		return c.Blob(http.StatusOK, "application/yaml", swaggerYAMLBytes)
	})
	e.GET("/api/docs", func(c echo.Context) error {
		return c.HTML(http.StatusOK, swaggerUIPage)
	})

	// Create handlers
	nodeHandler := handlers.NewNodeHandler(cfg.NodeStore, cfg.CommandStore, cfg.GroupStore, hub, regToken, cfg.AuroraBootURL)
	cmdHandler := handlers.NewCommandHandler(cfg.CommandStore, cfg.NodeStore, hub)
	artifactHandler := handlers.NewArtifactHandler(cfg.Builder, cfg.ArtifactStore, cfg.GroupStore, cfg.SecureBootKeySetStore, cfg.ArtifactsDir, regToken, cfg.AuroraBootURL)
	groupHandler := handlers.NewGroupHandler(cfg.GroupStore)
	settingsHandler := handlers.NewSettingsHandler(&regToken, cfg.RegTokenFile)

	// WebSocket handlers
	agentWSHandler := &ws.AgentHandler{
		Hub:      hub,
		Nodes:    cfg.NodeStore,
		Commands: cfg.CommandStore,
	}
	uiWSHandler := &ws.UIHandler{Hub: hub}

	// Public endpoints
	e.GET("/api/v1/install-agent", nodeHandler.InstallScript)

	// WebSocket endpoints (agent auth via query param token)
	e.GET("/api/v1/ws", agentWSHandler.HandleAgentWS)

	// Agent registration (registration token auth)
	regGroup := e.Group("/api/v1/nodes")
	regGroup.Use(auth.RegistrationTokenAuth(&regToken))
	regGroup.POST("/register", nodeHandler.Register)

	// Agent-only node endpoints (node API key auth). RequireNodeMatch binds
	// every route to the authenticated node's identity: the :nodeID in the path
	// must equal the node that the API key belongs to, so a registered node can
	// only act on its own resources (prevents node-impersonation / BOLA).
	agentGroup := e.Group("/api/v1/nodes/:nodeID")
	agentGroup.Use(auth.NodeAPIKeyMiddleware(cfg.NodeStore))
	agentGroup.Use(auth.RequireNodeMatch)
	agentGroup.POST("/heartbeat", nodeHandler.Heartbeat)

	// Shared command routes used by BOTH the agent (poll/report) and the
	// admin/UI (inspect/override). Registered ONCE under AgentOrAdminMiddleware:
	// registering them on two separate auth groups makes Echo silently shadow
	// the first, which previously routed every agent poll into the admin-only
	// handler (401). The handlers branch on the authenticated identity
	// (auth.AuthNodeID): agents are node-scoped (own commands only), admins act
	// across nodes.
	sharedCmd := e.Group("/api/v1/nodes/:nodeID")
	sharedCmd.Use(auth.AgentOrAdminMiddleware(cfg.AdminPassword, cfg.NodeStore))
	sharedCmd.GET("/commands", nodeHandler.GetCommands)
	sharedCmd.PUT("/commands/:commandID/status", cmdHandler.UpdateStatus)

	// Admin/UI endpoints (admin password auth)
	adminGroup := e.Group("/api/v1")
	adminGroup.Use(auth.AdminMiddleware(cfg.AdminPassword))

	// Node management
	adminGroup.GET("/nodes", nodeHandler.List)
	adminGroup.GET("/nodes/:nodeID", nodeHandler.Get)
	adminGroup.DELETE("/nodes/:nodeID", nodeHandler.Delete)
	adminGroup.POST("/nodes/:nodeID/decommission", nodeHandler.Decommission)
	adminGroup.PUT("/nodes/:nodeID/labels", nodeHandler.SetLabels)
	adminGroup.PUT("/nodes/:nodeID/group", nodeHandler.SetGroup)
	// GET /nodes/:nodeID/commands and PUT .../commands/:commandID/status are
	// served by the shared agent-or-admin group above (single registration to
	// avoid Echo route shadowing); they branch on the caller's identity.
	adminGroup.POST("/nodes/:nodeID/commands", cmdHandler.Create)
	adminGroup.DELETE("/nodes/:nodeID/commands/:commandID", cmdHandler.Delete)
	adminGroup.DELETE("/nodes/:nodeID/commands", cmdHandler.ClearHistory)
	adminGroup.POST("/nodes/commands", cmdHandler.CreateBulk)

	// Group management
	adminGroup.POST("/groups", groupHandler.Create)
	adminGroup.GET("/groups", groupHandler.List)
	adminGroup.GET("/groups/:id", groupHandler.Get)
	adminGroup.PUT("/groups/:id", groupHandler.Update)
	adminGroup.DELETE("/groups/:id", groupHandler.Delete)
	adminGroup.POST("/groups/:id/commands", cmdHandler.CreateForGroup)

	// Artifact management
	adminGroup.POST("/artifacts/upload-overlay", artifactHandler.UploadOverlay)
	adminGroup.POST("/artifacts", artifactHandler.Create)
	adminGroup.GET("/artifacts", artifactHandler.List)
	adminGroup.DELETE("/artifacts/failed", artifactHandler.ClearFailed)
	adminGroup.GET("/artifacts/:id", artifactHandler.Get)
	adminGroup.GET("/artifacts/:id/logs", artifactHandler.GetLogs)
	adminGroup.POST("/artifacts/:id/cancel", artifactHandler.Cancel)
	adminGroup.PATCH("/artifacts/:id", artifactHandler.Update)
	adminGroup.DELETE("/artifacts/:id", artifactHandler.Delete)

	// Artifact downloads — accepts admin password OR node API key.
	// Registered before the admin group catches them, using inline middleware.
	dlAuth := auth.DownloadMiddleware(cfg.AdminPassword, cfg.NodeStore)
	e.GET("/api/v1/artifacts/:id/download/*", artifactHandler.Download, dlAuth)
	e.GET("/api/v1/artifacts/:id/image", artifactHandler.ExportImage, dlAuth)

	// UI WebSocket (admin auth)
	adminGroup.GET("/ws/ui", uiWSHandler.HandleUIWS)

	// Settings
	adminGroup.GET("/settings/registration-token", settingsHandler.GetRegistrationToken)
	adminGroup.POST("/settings/registration-token/rotate", settingsHandler.RotateRegistrationToken)

	// SecureBoot key management
	sbHandler := handlers.NewSecureBootHandler(cfg.SecureBootKeySetStore, cfg.KeysDir)
	adminGroup.POST("/secureboot-keys/generate", sbHandler.GenerateKeys)
	adminGroup.GET("/secureboot-keys", sbHandler.ListKeys)
	adminGroup.GET("/secureboot-keys/:id/export", sbHandler.ExportKeys)
	adminGroup.POST("/secureboot-keys/import", sbHandler.ImportKeys)
	adminGroup.DELETE("/secureboot-keys/:id", sbHandler.DeleteKeys)

	// Deploy hub
	if cfg.DeploymentStore != nil {
		deployHandler := handlers.NewDeployHandler(cfg.ArtifactStore, cfg.DeploymentStore, cfg.BMCTargetStore, cfg.NetbootManager, cfg.ArtifactsDir, cfg.ISOServe, hub).
			WithBaseContext(cfg.BaseContext)
		adminGroup.POST("/netboot/start", deployHandler.StartNetboot)
		adminGroup.POST("/netboot/stop", deployHandler.StopNetboot)
		adminGroup.GET("/netboot/status", deployHandler.NetbootStatus)
		adminGroup.POST("/artifacts/:id/deploy/redfish", deployHandler.DeployRedfish)
		adminGroup.POST("/bmc-targets", deployHandler.CreateBMCTarget)
		adminGroup.GET("/bmc-targets", deployHandler.ListBMCTargets)
		adminGroup.PUT("/bmc-targets/:id", deployHandler.UpdateBMCTarget)
		adminGroup.DELETE("/bmc-targets/:id", deployHandler.DeleteBMCTarget)
		adminGroup.POST("/bmc-targets/:id/inspect", deployHandler.InspectHardware)
		adminGroup.GET("/deployments", deployHandler.ListDeployments)
		adminGroup.GET("/deployments/:id", deployHandler.GetDeployment)
	}

	// SPA static files - serve from embedded UI assets
	setupSPA(e)

	return e
}

// setupSPA configures the Echo server to serve the SPA frontend.
// It serves static files from the embedded UI assets and falls back to index.html
// for any unmatched route that accepts text/html (SPA client-side routing).
func setupSPA(e *echo.Echo) {
	// Get the dist subdirectory from the embedded FS
	distFS, err := fs.Sub(ui.Assets, "dist")
	if err != nil {
		return
	}
	httpFS := http.FS(distFS)

	// Serve static files
	e.GET("/assets/*", echo.WrapHandler(http.FileServer(httpFS)))

	// SPA fallback: serve index.html for any unmatched route that accepts text/html
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip API routes
			if strings.HasPrefix(c.Request().URL.Path, "/api/") {
				return next(c)
			}

			// Try to serve the file from the filesystem
			path := c.Request().URL.Path
			if path == "/" {
				path = "/index.html"
			}

			// Check if the file exists in the embedded FS
			f, err := distFS.Open(strings.TrimPrefix(path, "/"))
			if err == nil {
				f.Close()
				// File exists, serve it
				http.FileServer(httpFS).ServeHTTP(c.Response(), c.Request())
				return nil
			}

			// If the request accepts HTML, serve index.html (SPA fallback)
			accept := c.Request().Header.Get("Accept")
			if strings.Contains(accept, "text/html") {
				indexFile, err := distFS.Open("index.html")
				if err != nil {
					return next(c)
				}
				defer indexFile.Close()

				stat, err := indexFile.Stat()
				if err != nil {
					return next(c)
				}

				rs, ok := indexFile.(io.ReadSeeker)
				if !ok {
					return next(c)
				}
				http.ServeContent(c.Response(), c.Request(), "index.html", stat.ModTime(), rs)
				return nil
			}

			return next(c)
		}
	})
}
