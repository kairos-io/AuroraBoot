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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/kairos-io/AuroraBoot/docs" // swag-generated; populates docs.SwaggerInfo
	"github.com/kairos-io/AuroraBoot/internal/builder/auroraboot"
	netbootmgr "github.com/kairos-io/AuroraBoot/internal/netbootmgr"
	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/server"
	"github.com/kairos-io/AuroraBoot/pkg/ws"

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

	store, err := gormstore.New(dbDSN)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	artifactStore := &gormstore.ArtifactStoreAdapter{S: store}

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

	netbootManager := netbootmgr.NewManager()

	e := server.New(server.Config{
		NodeStore:             nodeStore,
		CommandStore:          commandStore,
		GroupStore:            groupStore,
		ArtifactStore:         artifactStore,
		SecureBootKeySetStore: secureBootKeySetStore,
		NetbootManager:        netbootManager,
		DeploymentStore:       deploymentStore,
		BMCTargetStore:        bmcTargetStore,
		Builder:               artifactBuilder,
		AdminPassword:         adminPassword,
		RegToken:              regToken,
		RegTokenFile:          regTokenFile,
		AuroraBootURL:           externalURL,
		ArtifactsDir:          artifactsDir,
		KeysDir:               keysDir,
		Hub:                   wsHub,
	})

	fmt.Fprintf(os.Stderr, "AuroraBoot fleet server starting on %s\n", listenAddr)
	fmt.Fprintf(os.Stderr, "  UI:        %s\n", externalURL)
	fmt.Fprintf(os.Stderr, "  DB:        %s\n", dbDSN)
	fmt.Fprintf(os.Stderr, "  Artifacts: %s\n", artifactsDir)

	return e.Start(listenAddr)
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

func generateToken(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Failed to generate token: %v", err)
	}
	return hex.EncodeToString(b)
}
