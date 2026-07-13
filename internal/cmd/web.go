// AuroraBoot fleet management server. This file is also the entry point
// for swag-generated OpenAPI documentation.
//
//	@title			AuroraBoot API
//	@version		0.1.0
//	@description	HTTP API for the AuroraBoot Kairos node manager.
//	@description
//	@description	Three security schemes are used across the API:
//	@description	  * AdminBearer — Authorization: Bearer <admin-password>. Human/UI/automation callers.
//	@description	  * NodeAPIKey — Authorization: Bearer <api-key>. Registered Kairos nodes calling their own endpoints. The api-key is returned from POST /api/v1/nodes/register.
//	@description	  * RegistrationToken — shared secret carried inside the body of POST /api/v1/nodes/register.
//	@description
//	@description	Two WebSocket endpoints also exist but are not described in this spec:
//	@description	  * GET /api/v1/ws — agent command channel (auth via ?token=<api-key>).
//	@description	  * GET /api/v1/ws/ui — UI live update channel (auth via ?token=<admin-password>).
//
//	@contact.name	Kairos authors
//	@contact.url	https://github.com/kairos-io/AuroraBoot
//	@license.name	Apache-2.0
//	@license.url	https://www.apache.org/licenses/LICENSE-2.0
//
//	@BasePath		/
//
//	@securityDefinitions.apikey	AdminBearer
//	@in							header
//	@name						Authorization
//	@description				Supply as "Bearer <admin-password>".
//
//	@securityDefinitions.apikey	NodeAPIKey
//	@in							header
//	@name						Authorization
//	@description				Supply as "Bearer <api-key>" returned from POST /api/v1/nodes/register.
//
//	@tag.name		Health
//	@tag.description	Liveness and readiness probes.
//	@tag.name		Agent bootstrap
//	@tag.description	Public endpoints a brand-new Kairos node uses to join the fleet.
//	@tag.name		Agent
//	@tag.description	Endpoints a registered node uses with its api-key.
//	@tag.name		Nodes
//	@tag.description	Fleet read and admin operations.
//	@tag.name		Groups
//	@tag.description	Logical groups for nodes.
//	@tag.name		Commands
//	@tag.description	Queued remote operations (upgrade, reset, exec...).
//	@tag.name		Artifacts
//	@tag.description	Image builds produced by the embedded AuroraBoot.
//	@tag.name		SecureBoot
//	@tag.description	UKI signing key sets.
//	@tag.name		Settings
//	@tag.description	Instance-level settings (registration token, ...).
package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/kairos-io/AuroraBoot/docs" // swag-generated; populates docs.SwaggerInfo
	"github.com/kairos-io/AuroraBoot/internal/builder/auroraboot"
	netbootmgr "github.com/kairos-io/AuroraBoot/internal/netbootmgr"
	"github.com/kairos-io/AuroraBoot/internal/secrets"
	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/isoserve"
	"github.com/kairos-io/AuroraBoot/pkg/server"
	"github.com/kairos-io/AuroraBoot/pkg/ws"

	"github.com/labstack/echo/v4"
	"github.com/urfave/cli/v2"
)

// WebCMD launches the fleet management server: REST API, React UI, agent
// and UI WebSockets, GORM-backed store, build runner, install-agent
// bootstrap script. This replaces the historical Alpine.js + jobstorage
// `web` subcommand wholesale.
var WebCMD = cli.Command{
	Name:    "web",
	Aliases: []string{"w"},
	Usage:   "Run the fleet management web UI and REST API",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "listen", Value: ":8080", Usage: "HTTP listen address"},
		&cli.StringFlag{Name: "data-dir", Value: "./data", Usage: "Base directory for all AuroraBoot data (db, artifacts, keys)"},
		&cli.StringFlag{Name: "db", Usage: "Database DSN (default: <data-dir>/auroraboot.db)"},
		&cli.StringFlag{Name: "artifacts-dir", Usage: "Directory for built artifacts (default: <data-dir>/artifacts)"},
		&cli.StringFlag{Name: "admin-password", Usage: "Admin password for UI access", EnvVars: []string{"AURORABOOT_ADMIN_PASSWORD"}},
		&cli.StringFlag{Name: "registration-token", Usage: "Registration token for node enrollment", EnvVars: []string{"AURORABOOT_REG_TOKEN"}},
		&cli.StringFlag{Name: "url", Usage: "External URL of this AuroraBoot instance (for cloud-config injection)", EnvVars: []string{"AURORABOOT_URL"}},
		&cli.StringFlag{Name: "tls-cert", Usage: "Path to the TLS certificate for the API server. Both --tls-cert and --tls-key must be set to enable HTTPS", EnvVars: []string{"AURORABOOT_TLS_CERT"}},
		&cli.StringFlag{Name: "tls-key", Usage: "Path to the TLS private key for the API server. Both --tls-cert and --tls-key must be set to enable HTTPS", EnvVars: []string{"AURORABOOT_TLS_KEY"}},
		&cli.StringFlag{Name: "redfish-serve-url", Usage: "Advertised base URL a BMC fetches an artifact ISO from for Redfish deploys (default: --url). The BMC management network may differ from the UI network", EnvVars: []string{"AURORABOOT_REDFISH_SERVE_URL"}},
		&cli.StringFlag{Name: "redfish-serve-addr", Usage: "Bind address for the Redfish ISO-serve (e.g. 10.0.0.5:8090). Required to enable serving local artifact ISOs to a BMC", EnvVars: []string{"AURORABOOT_REDFISH_SERVE_ADDR"}},
		&cli.StringFlag{Name: "redfish-serve-tls-cert", Usage: "TLS certificate for the Redfish ISO-serve (opt-in HTTPS; requires a BMC-trusted cert)"},
		&cli.StringFlag{Name: "redfish-serve-tls-key", Usage: "TLS key for the Redfish ISO-serve"},
		&cli.StringFlag{Name: "redfish-quirks-dir", Usage: "Directory of operator-supplied *.yaml/*.yml Redfish quirk profiles, loaded once at server start (not hot-reloaded). A BMCTarget's vendor resolves to a profile by name; an operator profile named the same as a built-in overrides it (logged). A malformed profile is skipped, not fatal", EnvVars: []string{redfishQuirksDirEnv}},
	},
	Action: runWeb,
}

func runWeb(c *cli.Context) error {
	listenAddr := c.String("listen")
	dataDir := c.String("data-dir")
	dbDSN := c.String("db")
	artifactsDir := c.String("artifacts-dir")
	adminPassword := c.String("admin-password")
	regToken := c.String("registration-token")
	externalURL := c.String("url")
	tlsCert := c.String("tls-cert")
	tlsKey := c.String("tls-key")
	redfishServeURL := c.String("redfish-serve-url")
	redfishServeAddr := c.String("redfish-serve-addr")
	redfishServeTLSCert := c.String("redfish-serve-tls-cert")
	redfishServeTLSKey := c.String("redfish-serve-tls-key")
	redfishQuirksDir := c.String("redfish-quirks-dir")

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if dbDSN == "" {
		dbDSN = filepath.Join(dataDir, "auroraboot.db")
	}
	if artifactsDir == "" {
		artifactsDir = filepath.Join(dataDir, "artifacts")
	}
	keysDir := filepath.Join(dataDir, "keys")

	// Resolve secrets in priority order: flag/env > persisted file > generate.
	// Persisted secrets live under <data-dir>/secrets/ so they survive restarts.
	secretsDir := filepath.Join(dataDir, "secrets")
	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return fmt.Errorf("create secrets directory: %w", err)
	}
	adminPasswordFile := filepath.Join(secretsDir, "admin-password")
	regTokenFile := filepath.Join(secretsDir, "registration-token")

	if adminPassword == "" {
		adminPassword = loadOrGenerateSecret(adminPasswordFile, "admin password")
	}
	if regToken == "" {
		regToken = loadOrGenerateSecret(regTokenFile, "registration token")
	}
	if externalURL == "" {
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "localhost"
		}
		externalURL = fmt.Sprintf("http://%s%s", hostname, listenAddr)
		fmt.Fprintf(os.Stderr, "Warning: --url not set, using %s\n", externalURL)
		fmt.Fprintf(os.Stderr, "  Set --url or AURORABOOT_URL for the externally reachable URL\n")
	}

	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		return fmt.Errorf("create artifacts directory: %w", err)
	}

	// Data encryption key for BMC credentials at rest (AES-256-GCM). Lives
	// alongside the other secrets as a 0600 file; generated on first run.
	bmcKeyFile := filepath.Join(secretsDir, "bmc-key")
	bmcCipher, err := secrets.LoadOrGenerateCipher(bmcKeyFile)
	if err != nil {
		return fmt.Errorf("load BMC encryption key: %w", err)
	}

	store, err := gormstore.New(dbDSN)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	store.WithCipher(bmcCipher)

	// Reconcile deployments left Active by a previous process: a restart orphans
	// their in-flight goroutine, so they can never complete. Mark them Failed.
	if err := handlers.ReconcileOrphanedDeployments(context.Background(), &gormstore.DeploymentStoreAdapter{S: store}); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: reconciling orphaned deployments: %v\n", err)
	}

	artifactStore := &gormstore.ArtifactStoreAdapter{S: store}

	// Reconcile artifacts left Pending or Building by a previous process: a
	// restart orphans their build goroutine, so they can never reach Ready on
	// their own. Mark them Error so the UI reflects a terminal state.
	if err := handlers.ReconcileOrphanedArtifacts(context.Background(), artifactStore); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: reconciling orphaned artifacts: %v\n", err)
	}

	// Create the WebSocket hub up front so the builder can broadcast log
	// chunks to subscribed UI clients as they arrive. The same hub is
	// handed to server.New below so the HTTP routes share it.
	wsHub := ws.NewHub()

	artifactBuilder := auroraboot.New(artifactsDir, nil, artifactStore).
		WithLogBroadcaster(wsHub.UI)

	nodeStore := &gormstore.NodeStoreAdapter{S: store}
	commandStore := &gormstore.CommandStoreAdapter{S: store}
	groupStore := &gormstore.GroupStoreAdapter{S: store}
	secureBootKeySetStore := &gormstore.SecureBootKeySetStoreAdapter{S: store}
	deploymentStore := &gormstore.DeploymentStoreAdapter{S: store}
	bmcTargetStore := &gormstore.BMCTargetStoreAdapter{S: store}
	settingsStore := &gormstore.SettingsStoreAdapter{S: store}

	netbootManager := netbootmgr.NewManager()

	// Optional Redfish ISO-serve: serves a local artifact ISO over a tokenized,
	// BMC-reachable URL so virtual-media (URL-pull) deploys work without an
	// operator-supplied imageUrl. It only starts when a bind address is given; the
	// advertised base defaults to --url but can differ (the BMC management network
	// is often not the UI network).
	var isoServe *isoserve.Server
	// serveURL is the advertised base URL a BMC fetches the served ISO from. It is
	// hoisted out of the start block so it can seed the runtime image-source
	// settings' advertised URL even before an operator overrides it.
	serveURL := redfishServeURL
	if serveURL == "" {
		serveURL = externalURL
	}
	if redfishServeAddr != "" {
		isoServe = isoserve.New(isoserve.Config{
			BaseURL:  serveURL,
			BindAddr: redfishServeAddr,
			CertFile: redfishServeTLSCert,
			KeyFile:  redfishServeTLSKey,
		})
		if err := isoServe.Start(context.Background()); err != nil {
			return fmt.Errorf("start redfish iso-serve: %w", err)
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = isoServe.Shutdown(shutdownCtx)
		}()
		fmt.Fprintf(os.Stderr, "  ISO-serve: %s (bind %s)\n", serveURL, redfishServeAddr)
	}

	// Load operator-supplied Redfish quirk profiles once at start (not
	// hot-reloaded: an in-flight deploy must never have its profile swapped). The
	// loaded registry becomes the process-wide default, so the Redfish deploy path
	// resolves a BMCTarget's vendor to a profile by name — operator profiles
	// included — inside redfish.NewDeployer. Each profile and each skipped file is
	// logged by the loader.
	if err := loadRedfishQuirksDir(redfishQuirksDir); err != nil {
		return err
	}

	// Root context for background deploy goroutines, cancelled when runWeb
	// returns so in-flight Redfish deploys are aborted on shutdown.
	baseCtx, baseCancel := context.WithCancel(context.Background())
	defer baseCancel()

	e := server.New(server.Config{
		BaseContext:           baseCtx,
		NodeStore:             nodeStore,
		CommandStore:          commandStore,
		GroupStore:            groupStore,
		ArtifactStore:         artifactStore,
		SecureBootKeySetStore: secureBootKeySetStore,
		NetbootManager:        netbootManager,
		DeploymentStore:       deploymentStore,
		BMCTargetStore:        bmcTargetStore,
		SettingsStore:         settingsStore,
		Builder:               artifactBuilder,
		AdminPassword:         adminPassword,
		RegToken:              regToken,
		RegTokenFile:          regTokenFile,
		AuroraBootURL:         externalURL,
		ArtifactsDir:          artifactsDir,
		KeysDir:               keysDir,
		Hub:                   wsHub,
		ISOServe:              isoServe,
		RedfishServeURL:       redfishServeURLSeed(isoServe, serveURL),
	})

	fmt.Fprintf(os.Stderr, "AuroraBoot fleet server starting on %s\n", listenAddr)
	fmt.Fprintf(os.Stderr, "  UI:        %s\n", externalURL)
	fmt.Fprintf(os.Stderr, "  DB:        %s\n", dbDSN)
	fmt.Fprintf(os.Stderr, "  Artifacts: %s\n", artifactsDir)

	return serve(e, listenAddr, tlsCert, tlsKey)
}

// serve starts the Echo server, choosing HTTPS when both a TLS certificate and
// key are provided and falling back to plaintext HTTP otherwise. When serving
// plaintext it logs a prominent warning, because all three bearer credentials
// (admin password, node API keys, registration token) and cloud-configs cross
// the wire in clear; a TLS cert/key or a TLS-terminating reverse proxy is
// strongly recommended for production. Plaintext is intentionally not a hard
// failure so dev and proxied deployments keep working.
//
// Both code paths use Echo's Start/StartTLS, which install the same signal
// handling and graceful-shutdown behaviour.
func serve(e *echo.Echo, listenAddr, tlsCert, tlsKey string) error {
	return serveWith(os.Stderr, listenAddr, tlsCert, tlsKey,
		func() error { return e.StartTLS(listenAddr, tlsCert, tlsKey) },
		func() error { return e.Start(listenAddr) })
}

// serveWith holds the TLS-vs-plaintext decision and the plaintext warning,
// independent of the concrete Echo server, so the decision can be unit-tested.
// It calls startTLS when both a cert and key are provided, otherwise it emits
// the plaintext warning to w and calls startPlain. The chosen starter's error
// is returned unchanged.
func serveWith(w io.Writer, listenAddr, tlsCert, tlsKey string, startTLS, startPlain func() error) error {
	if tlsCert != "" && tlsKey != "" {
		_, _ = fmt.Fprintf(w, "TLS enabled: serving HTTPS on %s using cert %s\n", listenAddr, tlsCert)
		return startTLS()
	}

	const warning = "\n" +
		"*****************************************************************************\n" +
		"WARNING: serving plaintext HTTP — TLS is NOT enabled.\n" +
		"  Admin password, node API keys, the registration token, and cloud-configs\n" +
		"  all traverse the network UNENCRYPTED. For production, set --tls-cert and\n" +
		"  --tls-key, or place AuroraBoot behind a TLS-terminating reverse proxy.\n" +
		"*****************************************************************************\n\n"
	_, _ = io.WriteString(w, warning)
	return startPlain()
}

// loadOrGenerateSecret reads the secret from path if it exists, otherwise
// generates a new one and persists it. The label is used in log messages.
func loadOrGenerateSecret(path, label string) string {
	if data, err := os.ReadFile(path); err == nil {
		secret := string(data)
		for len(secret) > 0 && (secret[len(secret)-1] == '\n' || secret[len(secret)-1] == '\r') {
			secret = secret[:len(secret)-1]
		}
		if secret != "" {
			return secret
		}
	}
	secret := generateToken(16)
	if err := os.WriteFile(path, []byte(secret), 0600); err != nil {
		log.Fatalf("Failed to persist %s: %v", label, err)
	}
	fmt.Fprintf(os.Stderr, "Generated %s: %s (saved to %s)\n", label, secret, path)
	return secret
}

// redfishServeURLSeed returns the advertised URL to seed the image-source
// settings with, but only when a local ISO-serve listener was actually
// configured. With no listener the advertised URL is irrelevant (local serving
// cannot be enabled), so seeding it would be misleading.
func redfishServeURLSeed(isoServe *isoserve.Server, serveURL string) string {
	if isoServe == nil {
		return ""
	}
	return serveURL
}

func generateToken(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Failed to generate token: %v", err)
	}
	return hex.EncodeToString(b)
}
