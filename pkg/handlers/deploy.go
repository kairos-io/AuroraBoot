package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/kairos-io/AuroraBoot/pkg/hardware"
	"github.com/kairos-io/AuroraBoot/pkg/redfish"
	netbootmgr "github.com/kairos-io/AuroraBoot/internal/netbootmgr"
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

	// Find an ISO file in the artifact's files list.
	isoPath := ""
	for _, f := range artifact.ArtifactFiles {
		if filepath.Ext(f) == ".iso" {
			isoPath = filepath.Join(h.artifactsDir, artifactID, f)
			break
		}
	}
	if isoPath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no ISO file found for this artifact"})
	}

	dep := &store.Deployment{
		ID:         uuid.New().String(),
		ArtifactID: artifactID,
		Method:     "redfish",
		Status:     store.DeployActive,
		Message:    "Deployment initiated",
		BMCTargetID: bmcID,
		Progress:   0,
		StartedAt:  time.Now(),
	}

	if err := h.deployments.Create(ctx, dep); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create deployment record"})
	}

	// Start async deployment goroutine.
	go h.runRedfishDeploy(dep.ID, isoPath, endpoint, username, password, vendor, verifySSL)

	return c.JSON(http.StatusAccepted, dep)
}

func (h *DeployHandler) runRedfishDeploy(deploymentID, isoPath, endpoint, username, password, vendor string, verifySSL bool) {
	ctx := fmt.Sprintf("deployment %s", deploymentID)

	client, err := redfish.NewVendorClient(redfish.VendorType(vendor), endpoint, username, password, verifySSL, 30*time.Minute)
	if err != nil {
		log.Printf("[%s] failed to create redfish client: %v", ctx, err)
		h.failDeployment(deploymentID, fmt.Sprintf("failed to create redfish client: %v", err))
		return
	}

	_, err = client.DeployISO(isoPath)
	if err != nil {
		log.Printf("[%s] DeployISO failed: %v", ctx, err)
		h.failDeployment(deploymentID, fmt.Sprintf("DeployISO failed: %v", err))
		return
	}

	// Poll deployment status every 10 seconds.
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	timeout := time.After(30 * time.Minute)

	for {
		select {
		case <-ticker.C:
			status, err := client.GetDeploymentStatus()
			if err != nil {
				log.Printf("[%s] failed to get deployment status: %v", ctx, err)
				continue
			}

			h.updateDeploymentProgress(deploymentID, status.Progress, status.Message)

			if status.State == "Completed" {
				h.completeDeployment(deploymentID, "Deployment completed successfully")
				return
			}
			if status.State == "Failed" {
				h.failDeployment(deploymentID, fmt.Sprintf("Deployment failed: %s", status.Message))
				return
			}
		case <-timeout:
			h.failDeployment(deploymentID, "deployment timed out after 30 minutes")
			return
		}
	}
}

func (h *DeployHandler) updateDeploymentProgress(id string, progress int, message string) {
	dep, err := h.deployments.GetByID(context.Background(), id)
	if err != nil {
		return
	}
	dep.Progress = progress
	dep.Message = message
	_ = h.deployments.Update(context.Background(), dep)
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

	client, err := redfish.NewVendorClient(
		redfish.VendorType(target.Vendor),
		target.Endpoint,
		target.Username,
		target.Password,
		target.VerifySSL,
		30*time.Second,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to create redfish client: %v", err)})
	}

	inspector := hardware.NewInspector(client)
	info, err := inspector.InspectSystem()
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
