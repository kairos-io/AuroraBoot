package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	netbootmgr "github.com/kairos-io/AuroraBoot/internal/netbootmgr"
	"github.com/kairos-io/AuroraBoot/pkg/hardware"
	"github.com/kairos-io/AuroraBoot/pkg/isoserve"
	"github.com/kairos-io/AuroraBoot/pkg/redfish"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/kairos-io/AuroraBoot/pkg/ws"
	"github.com/labstack/echo/v4"
)

// redfishDeployTimeout bounds the full virtual-media deploy flow (InsertMedia ->
// boot override -> reset -> Task poll).
const redfishDeployTimeout = 30 * time.Minute

// DeployHandler handles deployment-related REST endpoints.
type DeployHandler struct {
	artifacts    store.ArtifactStore
	deployments  store.DeploymentStore
	bmcTargets   store.BMCTargetStore
	netboot      *netbootmgr.Manager
	artifactsDir string
	// isoServe serves a local artifact ISO over a tokenized, BMC-reachable URL.
	// May be nil, in which case a Redfish deploy must supply an explicit imageUrl.
	isoServe *isoserve.Server

	// settings exposes the runtime image-source settings (global default image URL,
	// local-serve enable flag, advertised URL). May be nil (e.g. in tests or when
	// no settings store is wired), in which case the deploy falls back to the
	// request/per-BMC imageUrl or the configured local-serve.
	settings store.SettingsStore

	// hub is the WebSocket hub used to broadcast live deploy-step progress to UI
	// clients. May be nil (e.g. in tests or when no hub is wired), in which case
	// progress is only persisted to the Deployment row and surfaced via polling.
	hub *ws.Hub

	// baseCtx is the parent context for every background deploy goroutine. It is
	// tied to the server lifecycle so a shutdown cancels in-flight deploys. When
	// unset it defaults to context.Background().
	baseCtx context.Context

	// runs holds a cancel func per in-flight deployment so a caller (or shutdown)
	// can abort it. Guarded by runsMu.
	runsMu sync.Mutex
	runs   map[string]context.CancelFunc
}

// NewDeployHandler creates a new DeployHandler.
func NewDeployHandler(
	artifacts store.ArtifactStore,
	deployments store.DeploymentStore,
	bmcTargets store.BMCTargetStore,
	nb *netbootmgr.Manager,
	artifactsDir string,
	isoServe *isoserve.Server,
	hub *ws.Hub,
) *DeployHandler {
	return &DeployHandler{
		artifacts:    artifacts,
		deployments:  deployments,
		bmcTargets:   bmcTargets,
		netboot:      nb,
		artifactsDir: artifactsDir,
		isoServe:     isoServe,
		hub:          hub,
		baseCtx:      context.Background(),
		runs:         make(map[string]context.CancelFunc),
	}
}

// WithBaseContext sets the parent context for background deploy goroutines so a
// server shutdown cancels any in-flight Redfish deploy. Returns the handler for
// chaining.
func (h *DeployHandler) WithBaseContext(ctx context.Context) *DeployHandler {
	if ctx != nil {
		h.baseCtx = ctx
	}
	return h
}

// WithSettings wires the runtime settings store so DeployRedfish can resolve the
// global default image URL and the local-serve enable flag. Passing nil leaves
// the handler using only the request/per-BMC imageUrl and the launch-configured
// local-serve. Returns the handler for chaining.
func (h *DeployHandler) WithSettings(s store.SettingsStore) *DeployHandler {
	h.settings = s
	return h
}

// registerRun stores the cancel func for an in-flight deployment.
func (h *DeployHandler) registerRun(id string, cancel context.CancelFunc) {
	h.runsMu.Lock()
	defer h.runsMu.Unlock()
	h.runs[id] = cancel
}

// deregisterRun removes and returns the cancel func for a deployment.
func (h *DeployHandler) deregisterRun(id string) {
	h.runsMu.Lock()
	defer h.runsMu.Unlock()
	delete(h.runs, id)
}

// CancelRun aborts an in-flight deployment by id. It reports whether a matching
// run was found.
func (h *DeployHandler) CancelRun(id string) bool {
	h.runsMu.Lock()
	cancel, ok := h.runs[id]
	h.runsMu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

// ReconcileOrphanedDeployments fails every deployment still marked Active. A
// process restart orphans the goroutine driving an Active deployment, so on
// startup those rows can never reach a terminal state on their own; flip them to
// Failed with an explanatory message. Safe to call once during bootstrap.
func ReconcileOrphanedDeployments(ctx context.Context, deployments store.DeploymentStore) error {
	deps, err := deployments.List(ctx)
	if err != nil {
		return fmt.Errorf("listing deployments: %w", err)
	}
	for _, dep := range deps {
		if dep.Status != store.DeployActive {
			continue
		}
		now := time.Now()
		dep.Status = store.DeployFailed
		dep.Message = "interrupted by server restart"
		dep.CompletedAt = &now
		if err := deployments.Update(ctx, dep); err != nil {
			return fmt.Errorf("failing orphaned deployment %s: %w", dep.ID, err)
		}
	}
	return nil
}

// --- Netboot endpoints ---

type startNetbootRequest struct {
	ArtifactID string `json:"artifactId"`
}

// StartNetboot handles POST /api/v1/netboot/start.
func (h *DeployHandler) StartNetboot(c echo.Context) error {
	var req startNetbootRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.ArtifactID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "artifactId is required"})
	}

	if err := h.netboot.Start(h.artifactsDir, req.ArtifactID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, h.netboot.GetStatus())
}

// StopNetboot handles POST /api/v1/netboot/stop.
func (h *DeployHandler) StopNetboot(c echo.Context) error {
	if err := h.netboot.Stop(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, h.netboot.GetStatus())
}

// NetbootStatus handles GET /api/v1/netboot/status.
func (h *DeployHandler) NetbootStatus(c echo.Context) error {
	return c.JSON(http.StatusOK, h.netboot.GetStatus())
}

// --- RedFish deploy ---

type deployRedfishRequest struct {
	BMCTargetID string `json:"bmcTargetId"`
	// ImageURL is the HTTP(S) URL the BMC pulls the ISO from (VirtualMedia
	// InsertMedia is URL-pull). Optional: when empty, AuroraBoot serves the
	// artifact's on-disk ISO over an ephemeral tokenized URL (requires the server
	// to have a configured ISO-serve).
	ImageURL string `json:"imageUrl"`
	// Inline BMC credentials (used when bmcTargetId is empty).
	Endpoint  string `json:"endpoint"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Vendor    string `json:"vendor"`
	VerifySSL *bool  `json:"verifySSL"`
	// SystemID optionally pins the target ComputerSystem by its Redfish Id. When
	// set it takes precedence over a saved target's SystemID; required when the BMC
	// exposes more than one system. Mirrors the CLI's --system-id.
	SystemID string `json:"systemId"`
}

// imageURLUsesHTTPS reports whether an operator-supplied media URL is fetched
// over HTTPS, derived from its scheme. The InsertMedia TransferProtocolType must
// match the URL the BMC actually fetches, so this keeps the two consistent.
func imageURLUsesHTTPS(imageURL string) (bool, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(parsed.Scheme, "https"), nil
}

// resolveOperatorImageURL applies the operator-supplied image-URL precedence
// (per-deploy > per-BMC > global default), all model (a). It returns "" when no
// tier supplies a URL, signalling the caller to fall back to local serving
// (model b). It does not validate; the caller SSRF-validates the chosen URL.
func resolveOperatorImageURL(reqURL, bmcURL, defaultURL string) string {
	switch {
	case reqURL != "":
		return reqURL
	case bmcURL != "":
		return bmcURL
	default:
		return defaultURL
	}
}

// defaultImageURL returns the global default image URL setting, or "" when no
// settings store is wired or the key is unset. A read error is surfaced so the
// caller can fail closed rather than silently treating it as unset.
func (h *DeployHandler) defaultImageURL(ctx context.Context) (string, error) {
	if h.settings == nil {
		return "", nil
	}
	v, _, err := h.settings.Get(ctx, SettingDefaultImageURL)
	return v, err
}

// localServeEnabled reports whether the runtime local-serve enable flag is set.
// It is the gate (alongside a launch-configured listener) for serving the
// artifact ISO locally. Defaults to false: missing setting, no settings store, or
// a read error all yield false so deploys never silently serve without an
// explicit opt-in.
func (h *DeployHandler) localServeEnabled(ctx context.Context) bool {
	if h.settings == nil {
		return false
	}
	v, _, err := h.settings.Get(ctx, SettingLocalServeEnabled)
	if err != nil {
		return false
	}
	return v == "true"
}

// DeployRedfish handles POST /api/v1/artifacts/:id/deploy/redfish.
func (h *DeployHandler) DeployRedfish(c echo.Context) error {
	artifactID := c.Param("id")
	ctx := c.Request().Context()

	artifact, err := h.artifacts.GetByID(ctx, artifactID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found"})
	}

	var req deployRedfishRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	var (
		endpoint    string
		username    string
		password    string
		vendor      string
		verifySSL   bool
		systemID    string
		bmcImageURL string
		bmcID       string
	)

	if req.BMCTargetID != "" {
		target, err := h.bmcTargets.GetByID(ctx, req.BMCTargetID)
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "BMC target not found"})
		}
		endpoint = target.Endpoint
		username = target.Username
		password = target.Password
		vendor = target.Vendor
		verifySSL = target.VerifySSL
		systemID = target.SystemID
		bmcImageURL = target.ImageURL
		bmcID = target.ID
	} else {
		if req.Endpoint == "" || req.Username == "" || req.Password == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "endpoint, username, and password are required when bmcTargetId is not provided"})
		}
		endpoint = req.Endpoint
		username = req.Username
		password = req.Password
		vendor = req.Vendor
		if vendor == "" {
			vendor = "generic"
		}
		if req.VerifySSL != nil {
			verifySSL = *req.VerifySSL
		} else {
			verifySSL = true
		}
	}

	// An explicit request SystemID overrides the saved target's (and is the only
	// way to select a system for an inline-credentials deploy). When empty, fall
	// back to whatever the saved target carries.
	if req.SystemID != "" {
		systemID = req.SystemID
	}

	// Locate the artifact's on-disk ISO. InsertMedia is URL-pull (no byte
	// upload), so the BMC fetches the image from a URL.
	isoFile := ""
	for _, f := range artifact.ArtifactFiles {
		if filepath.Ext(f) == ".iso" {
			isoFile = f
			break
		}
	}
	if isoFile == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no ISO file found for this artifact"})
	}

	// Resolve the image URL the BMC will fetch, and whether the BMC must fetch it
	// over HTTPS. InsertMedia advertises a TransferProtocolType that must match the
	// served URL's scheme, so derive useHTTPS alongside the URL. Image-source
	// precedence (highest to lowest), each operator-supplied URL SSRF-validated as
	// defense in depth even when validated on write:
	//   1. req.ImageURL  — per-deploy override (model a).
	//   2. BMC ImageURL  — per-BMC override (model a).
	//   3. global defaultImageURL setting (model a).
	//   4. local serve: AuroraBoot serves the artifact's on-disk ISO over a
	//      tokenized, BMC-reachable URL (model b), only when a listener was
	//      configured at launch AND the runtime enable flag is set.
	//   5. otherwise 503.
	defaultURL, err := h.defaultImageURL(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to read image-source settings"})
	}
	imageURL := resolveOperatorImageURL(req.ImageURL, bmcImageURL, defaultURL)

	serveToken := ""
	useHTTPS := false
	if imageURL != "" {
		if err := isoserve.ValidateMediaURL(imageURL); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid imageUrl: %v", err)})
		}
		useHTTPS, err = imageURLUsesHTTPS(imageURL)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid imageUrl: %v", err)})
		}
	} else {
		// No operator-supplied URL at any tier: fall back to local serving, which
		// requires both a launch-configured listener and the runtime enable flag.
		if h.isoServe == nil || !h.localServeEnabled(ctx) {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"error": "local ISO serving is not enabled on this server; provide imageUrl with a URL the BMC can reach, set a default image URL, or enable AuroraBoot-served artifacts",
			})
		}
		// ArtifactFiles stores each file's path relative to the server's working
		// directory (e.g. "data/artifacts/<id>/foo.iso"). Resolve it to an
		// absolute path: isoserve.Register requires an absolute path, and
		// re-joining it with artifactsDir/artifactID would double the prefix into
		// a bogus path that does not exist.
		absISO, err := filepath.Abs(isoFile)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("resolving artifact ISO path: %v", err)})
		}
		servedURL, token, err := h.isoServe.Register(absISO, redfishDeployTimeout+5*time.Minute)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to serve artifact ISO: %v", err)})
		}
		imageURL = servedURL
		serveToken = token
		useHTTPS = h.isoServe.UsesTLS()
	}

	dep := &store.Deployment{
		ID:          uuid.New().String(),
		ArtifactID:  artifactID,
		Method:      "redfish",
		Status:      store.DeployActive,
		Message:     "Deployment initiated",
		BMCTargetID: bmcID,
		Progress:    0,
		StartedAt:   time.Now(),
	}

	if err := h.deployments.Create(ctx, dep); err != nil {
		// Don't leak a live serve token for a deployment that never started.
		if serveToken != "" && h.isoServe != nil {
			h.isoServe.Revoke(serveToken)
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create deployment record"})
	}

	// Start async deployment goroutine. The HTTP request context is request-scoped
	// and would be cancelled on return, so the goroutine derives its own context
	// from the server-lifecycle base context (cancelled on shutdown) plus the
	// deploy timeout. The cancel func is registered so the run can be aborted.
	runCtx, cancel := context.WithTimeout(h.baseCtx, redfishDeployTimeout)
	h.registerRun(dep.ID, cancel)

	go h.runRedfishDeploy(runCtx, cancel, dep.ID, imageURL, serveToken, endpoint, username, password, vendor, systemID, verifySSL, useHTTPS)

	return c.JSON(http.StatusAccepted, dep)
}

// runRedfishDeploy drives the gofish-backed virtual-media deployment in the
// background and records the outcome on the store.Deployment row. It never logs
// credentials. ctx is derived from the server-lifecycle base context plus the
// deploy timeout; cancel is its cancel func, deregistered and called on return so
// the run-registry never retains a finished deploy. useHTTPS sets the InsertMedia
// TransferProtocolType so it matches the scheme the BMC actually fetches over.
func (h *DeployHandler) runRedfishDeploy(ctx context.Context, cancel context.CancelFunc, deploymentID, imageURL, serveToken, endpoint, username, password, vendor, systemID string, verifySSL, useHTTPS bool) {
	logPrefix := fmt.Sprintf("deployment %s", deploymentID)

	defer cancel()
	defer h.deregisterRun(deploymentID)

	// Revoke the one-shot serve token once the deploy reaches any terminal state.
	// The BMC has finished fetching the ISO by the time the deploy returns, so the
	// capability URL no longer needs to be live.
	if serveToken != "" && h.isoServe != nil {
		defer h.isoServe.Revoke(serveToken)
	}

	deployer := redfish.NewDeployer(redfish.Config{
		Endpoint:  endpoint,
		Username:  username,
		Password:  password,
		Vendor:    redfish.VendorType(vendor),
		VerifySSL: verifySSL,
		SystemID:  systemID,
		Timeout:   redfishDeployTimeout,
	})

	if err := deployer.Connect(ctx); err != nil {
		log.Printf("[%s] connecting to redfish endpoint failed: %v", logPrefix, err)
		h.failDeployment(deploymentID, fmt.Sprintf("connecting to redfish endpoint: %v", err))
		return
	}
	// Always tear the session down (DELETE) on both success and error.
	defer func() { _ = deployer.Close() }()

	result, err := deployer.Deploy(ctx, redfish.DeployRequest{
		ImageURL:              imageURL,
		BootTarget:            redfish.BootTargetCd,
		BootMode:              redfish.BootModeUEFI,
		TransferProtocolHTTPS: useHTTPS,
		Progress:              h.progressUpdater(deploymentID),
	})
	if err != nil {
		log.Printf("[%s] redfish deploy failed: %v", logPrefix, err)
		h.failDeployment(deploymentID, fmt.Sprintf("redfish deploy failed: %v", err))
		return
	}

	msg := "Deployment completed successfully"
	if result.TaskState != "" {
		msg = fmt.Sprintf("Deployment completed (task state: %s)", result.TaskState)
	}
	h.completeDeployment(deploymentID, msg)
}

// broadcastProgress fans a live deploy-progress event to all connected UI
// clients over the existing UI WebSocket. It is best-effort and nil-safe: when
// no hub is wired the event is simply dropped, and clients fall back to polling
// the deployment store. Broadcast does its own marshalling and write fan-out, so
// this never blocks the deploy on a slow client. The payload is nested under
// "data" to match the existing UI message envelope ({"type":…,"data":…}).
func (h *DeployHandler) broadcastProgress(id, status, step, message string, progress int) {
	if h.hub == nil || h.hub.UI == nil {
		return
	}
	h.hub.UI.Broadcast(map[string]any{
		"type": "deploy-progress",
		"data": map[string]any{
			"deploymentId": id,
			"status":       status,
			"progress":     progress,
			"step":         step,
			"message":      message,
		},
	})
}

// progressUpdater returns a redfish progress callback that writes live step and
// percentage onto the Deployment row. It only advances Progress (never regresses)
// and leaves the terminal Completed/Failed write to the caller. A read/update
// failure is best-effort and silently ignored — progress is non-authoritative.
// Alongside the store write it broadcasts the step to UI clients so the
// Deployments page reflects progress in real time rather than only on poll.
func (h *DeployHandler) progressUpdater(id string) func(step string, percent int) {
	return func(step string, percent int) {
		dep, err := h.deployments.GetByID(context.Background(), id)
		if err != nil {
			return
		}
		if percent <= dep.Progress && dep.Message == step {
			return
		}
		if percent > dep.Progress {
			dep.Progress = percent
		}
		dep.Message = step
		_ = h.deployments.Update(context.Background(), dep)
		h.broadcastProgress(id, dep.Status, step, dep.Message, dep.Progress)
	}
}

func (h *DeployHandler) completeDeployment(id, message string) {
	dep, err := h.deployments.GetByID(context.Background(), id)
	if err != nil {
		return
	}
	now := time.Now()
	dep.Status = store.DeployCompleted
	dep.Message = message
	dep.Progress = 100
	dep.CompletedAt = &now
	_ = h.deployments.Update(context.Background(), dep)
	h.broadcastProgress(id, dep.Status, "completed", message, dep.Progress)
}

func (h *DeployHandler) failDeployment(id, message string) {
	dep, err := h.deployments.GetByID(context.Background(), id)
	if err != nil {
		return
	}
	now := time.Now()
	dep.Status = store.DeployFailed
	dep.Message = message
	dep.CompletedAt = &now
	_ = h.deployments.Update(context.Background(), dep)
	h.broadcastProgress(id, dep.Status, "failed", message, dep.Progress)
}

// --- Hardware inspection ---

type inspectResponse struct {
	MemoryGiB      int    `json:"memoryGiB"`
	ProcessorCount int    `json:"processorCount"`
	Model          string `json:"model"`
	Manufacturer   string `json:"manufacturer"`
	SerialNumber   string `json:"serialNumber"`
	// SupportedFeatures lists the capabilities AuroraBoot positively detected for
	// this system (e.g. "UEFI", "SecureBoot"). It is informational: the API does
	// not gate on required features (the CLI does). A capability AuroraBoot could
	// not determine is simply absent from this list.
	SupportedFeatures []string `json:"supportedFeatures"`
}

// InspectHardware handles POST /api/v1/bmc-targets/:id/inspect.
func (h *DeployHandler) InspectHardware(c echo.Context) error {
	id := c.Param("id")
	ctx := c.Request().Context()

	target, err := h.bmcTargets.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "BMC target not found"})
	}

	deployer := redfish.NewDeployer(redfish.Config{
		Endpoint:  target.Endpoint,
		Username:  target.Username,
		Password:  target.Password,
		Vendor:    redfish.VendorType(target.Vendor),
		VerifySSL: target.VerifySSL,
		SystemID:  target.SystemID,
		Timeout:   30 * time.Second,
	})
	if err := deployer.Connect(ctx); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to connect to redfish endpoint: %v", err)})
	}
	// Always tear the session down (DELETE) on both success and error.
	defer func() { _ = deployer.Close() }()

	inspector := hardware.NewInspector(deployer)
	info, err := inspector.InspectSystem(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to inspect hardware: %v", err)})
	}

	return c.JSON(http.StatusOK, inspectResponse{
		MemoryGiB:         info.MemoryGiB,
		ProcessorCount:    info.ProcessorCount,
		Model:             info.Model,
		Manufacturer:      info.Manufacturer,
		SerialNumber:      info.SerialNumber,
		SupportedFeatures: sortedFeatures(info.Features),
	})
}

// sortedFeatures returns the detected feature names as a stable, sorted slice for
// a deterministic JSON response.
func sortedFeatures(features map[string]bool) []string {
	names := make([]string, 0, len(features))
	for name, present := range features {
		if present {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// --- BMC Target CRUD ---

// CreateBMCTarget handles POST /api/v1/bmc-targets.
func (h *DeployHandler) CreateBMCTarget(c echo.Context) error {
	var target store.BMCTarget
	if err := c.Bind(&target); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if target.Endpoint == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "endpoint is required"})
	}
	// SSRF-validate a per-BMC image-URL override on write (and again at deploy).
	if target.ImageURL != "" {
		if err := isoserve.ValidateMediaURL(target.ImageURL); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid imageUrl: %v", err)})
		}
	}

	if err := h.bmcTargets.Create(c.Request().Context(), &target); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create BMC target"})
	}
	return c.JSON(http.StatusCreated, target)
}

// ListBMCTargets handles GET /api/v1/bmc-targets.
func (h *DeployHandler) ListBMCTargets(c echo.Context) error {
	targets, err := h.bmcTargets.List(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list BMC targets"})
	}
	return c.JSON(http.StatusOK, targets)
}

// UpdateBMCTarget handles PUT /api/v1/bmc-targets/:id.
func (h *DeployHandler) UpdateBMCTarget(c echo.Context) error {
	id := c.Param("id")
	ctx := c.Request().Context()

	existing, err := h.bmcTargets.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "BMC target not found"})
	}

	var updated store.BMCTarget
	if err := c.Bind(&updated); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	// SSRF-validate a per-BMC image-URL override on write (and again at deploy).
	if updated.ImageURL != "" {
		if err := isoserve.ValidateMediaURL(updated.ImageURL); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid imageUrl: %v", err)})
		}
	}

	existing.Name = updated.Name
	existing.Endpoint = updated.Endpoint
	existing.Vendor = updated.Vendor
	existing.Username = updated.Username
	if updated.Password != "" {
		existing.Password = updated.Password
	}
	existing.VerifySSL = updated.VerifySSL
	existing.SystemID = updated.SystemID
	existing.ImageURL = updated.ImageURL
	existing.NodeID = updated.NodeID

	if err := h.bmcTargets.Update(ctx, existing); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update BMC target"})
	}
	return c.JSON(http.StatusOK, existing)
}

// DeleteBMCTarget handles DELETE /api/v1/bmc-targets/:id.
func (h *DeployHandler) DeleteBMCTarget(c echo.Context) error {
	id := c.Param("id")
	if err := h.bmcTargets.Delete(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete BMC target"})
	}
	return c.NoContent(http.StatusNoContent)
}

// --- Deployment history ---

// ListDeployments handles GET /api/v1/deployments.
func (h *DeployHandler) ListDeployments(c echo.Context) error {
	deps, err := h.deployments.List(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list deployments"})
	}
	return c.JSON(http.StatusOK, deps)
}

// GetDeployment handles GET /api/v1/deployments/:id.
func (h *DeployHandler) GetDeployment(c echo.Context) error {
	id := c.Param("id")
	dep, err := h.deployments.GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "deployment not found"})
	}
	return c.JSON(http.StatusOK, dep)
}
