package server

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/kairos-io/AuroraBoot/docs"
	"github.com/kairos-io/AuroraBoot/pkg/auth"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	netbootpkg "github.com/kairos-io/AuroraBoot/internal/netbootmgr"
	"github.com/kairos-io/AuroraBoot/internal/ui"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/kairos-io/AuroraBoot/pkg/ws"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Config holds all dependencies needed by the server.
type Config struct {
	NodeStore      store.NodeStore
	CommandStore   store.CommandStore
	GroupStore     store.GroupStore
	ArtifactStore          store.ArtifactStore
	SecureBootKeySetStore  store.SecureBootKeySetStore
	NetbootManager         *netbootpkg.Manager
	DeploymentStore        store.DeploymentStore
	BMCTargetStore         store.BMCTargetStore
	Builder                builder.ArtifactBuilder
	AdminPassword          string
	RegToken       string
	RegTokenFile   string // path where reg token is persisted (for rotation)
	AuroraBootURL    string
	ArtifactsDir   string
	KeysDir        string  // base directory for SecureBoot key sets
	Hub            *ws.Hub // optional, created if nil
}

// New creates and configures an Echo server with all routes and middleware.
func New(cfg Config) *echo.Echo {
	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.Logger())
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

	// Agent node endpoints (node API key auth)
	agentGroup := e.Group("/api/v1/nodes/:nodeID")
	agentGroup.Use(auth.NodeAPIKeyMiddleware(cfg.NodeStore))
	agentGroup.POST("/heartbeat", nodeHandler.Heartbeat)
	agentGroup.GET("/commands", nodeHandler.GetCommands)
	agentGroup.PUT("/commands/:commandID/status", cmdHandler.UpdateStatus)

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
	adminGroup.GET("/nodes/:nodeID/commands", nodeHandler.GetCommands)
	adminGroup.POST("/nodes/:nodeID/commands", cmdHandler.Create)
	adminGroup.PUT("/nodes/:nodeID/commands/:commandID/status", cmdHandler.UpdateStatus)
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
		deployHandler := handlers.NewDeployHandler(cfg.ArtifactStore, cfg.DeploymentStore, cfg.BMCTargetStore, cfg.NetbootManager, cfg.ArtifactsDir)
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
