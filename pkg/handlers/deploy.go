package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	netbootmgr "github.com/kairos-io/AuroraBoot/internal/netbootmgr"
	"github.com/kairos-io/AuroraBoot/pkg/hardware"
	"github.com/kairos-io/AuroraBoot/pkg/redfish"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// DeployHandler handles deployment-related REST endpoints.
type DeployHandler struct {
	artifacts    store.ArtifactStore
	deployments  store.DeploymentStore
	bmcTargets   store.BMCTargetStore
	netboot      *netbootmgr.Manager
	artifactsDir string
}

// NewDeployHandler creates a new DeployHandler.
func NewDeployHandler(
	artifacts store.ArtifactStore,
	deployments store.DeploymentStore,
	bmcTargets store.BMCTargetStore,
	nb *netbootmgr.Manager,
	artifactsDir string,
) *DeployHandler {
	return &DeployHandler{
		artifacts:    artifacts,
		deployments:  deployments,
		bmcTargets:   bmcTargets,
		netboot:      nb,
		artifactsDir: artifactsDir,
	}
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
	// InsertMedia is URL-pull). Required: serving a local on-disk ISO over an
	// ephemeral tokenized URL is Phase 1b (kairos-io/kairos#4111).
	ImageURL string `json:"imageUrl"`
	// Inline BMC credentials (used when bmcTargetId is empty).
	Endpoint  string `json:"endpoint"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Vendor    string `json:"vendor"`
	VerifySSL *bool  `json:"verifySSL"`
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
		endpoint  string
		username  string
		password  string
		vendor    string
		verifySSL bool
		bmcID     string
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

	// Confirm the artifact actually has an ISO. The bytes are not uploaded:
	// InsertMedia is URL-pull, so the BMC fetches the image from req.ImageURL.
	hasISO := false
	for _, f := range artifact.ArtifactFiles {
		if filepath.Ext(f) == ".iso" {
			hasISO = true
			break
		}
	}
	if !hasISO {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no ISO file found for this artifact"})
	}

	// VirtualMedia InsertMedia is URL-pull (no byte upload). Serving the
	// artifact's local ISO over an ephemeral, BMC-reachable URL is Phase 1b
	// (kairos-io/kairos#4111); until then an explicit imageUrl is required.
	if req.ImageURL == "" {
		return c.JSON(http.StatusNotImplemented, map[string]string{
			"error": "serving a local artifact ISO is not yet implemented (Phase 1b, kairos-io/kairos#4111); provide imageUrl with a URL the BMC can reach",
		})
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
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create deployment record"})
	}

	// Start async deployment goroutine. The HTTP request context is request-scoped
	// and would be cancelled on return, so the goroutine builds its own context.
	go h.runRedfishDeploy(dep.ID, req.ImageURL, endpoint, username, password, vendor, verifySSL)

	return c.JSON(http.StatusAccepted, dep)
}

// runRedfishDeploy drives the gofish-backed virtual-media deployment in the
// background and records the outcome on the store.Deployment row. It never logs
// credentials.
//
// TODO(#4111): there is no cancellable run-registry yet, so an in-flight deploy
// cannot be aborted and a process restart leaves the Active row orphaned. A run
// registry plus a startup reconciler that fails dangling Active deployments is
// Phase 1b.
func (h *DeployHandler) runRedfishDeploy(deploymentID, imageURL, endpoint, username, password, vendor string, verifySSL bool) {
	logPrefix := fmt.Sprintf("deployment %s", deploymentID)

	// The whole flow (InsertMedia -> boot override -> reset -> Task poll) is
	// bounded by a 30-minute deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	deployer := redfish.NewDeployer(redfish.Config{
		Endpoint:  endpoint,
		Username:  username,
		Password:  password,
		Vendor:    redfish.VendorType(vendor),
		VerifySSL: verifySSL,
		Timeout:   30 * time.Minute,
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
		TransferProtocolHTTPS: false,
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
}

// --- Hardware inspection ---

type inspectResponse struct {
	MemoryGiB      int    `json:"memoryGiB"`
	ProcessorCount int    `json:"processorCount"`
	Model          string `json:"model"`
	Manufacturer   string `json:"manufacturer"`
	SerialNumber   string `json:"serialNumber"`
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
		MemoryGiB:      info.MemoryGiB,
		ProcessorCount: info.ProcessorCount,
		Model:          info.Model,
		Manufacturer:   info.Manufacturer,
		SerialNumber:   info.SerialNumber,
	})
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

	existing.Name = updated.Name
	existing.Endpoint = updated.Endpoint
	existing.Vendor = updated.Vendor
	existing.Username = updated.Username
	if updated.Password != "" {
		existing.Password = updated.Password
	}
	existing.VerifySSL = updated.VerifySSL
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
