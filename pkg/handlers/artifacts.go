package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"
)

// ArtifactHandler handles artifact-related REST endpoints.
type ArtifactHandler struct {
	builder        builder.ArtifactBuilder
	store          store.ArtifactStore
	groups         store.GroupStore
	secureBootKeys store.SecureBootKeySetStore
	regToken       string
	aurorabootURL    string
	artifactsDir   string
}

// NewArtifactHandler creates a new ArtifactHandler.
func NewArtifactHandler(b builder.ArtifactBuilder, artifactStore store.ArtifactStore, groups store.GroupStore, secureBootKeys store.SecureBootKeySetStore, artifactsDir string, regToken string, aurorabootURL string) *ArtifactHandler {
	return &ArtifactHandler{
		builder:        b,
		store:          artifactStore,
		groups:         groups,
		secureBootKeys: secureBootKeys,
		regToken:       regToken,
		aurorabootURL:    aurorabootURL,
		artifactsDir:   artifactsDir,
	}
}

// createArtifactRequest is the expected body for creating an artifact build.
type createArtifactRequest struct {
	Name              string `json:"name"`
	BaseImage         string `json:"baseImage"`
	KairosVersion     string `json:"kairosVersion"`
	Model             string `json:"model"`
	Arch              string `json:"arch"`
	Variant           string `json:"variant"`
	KubernetesDistro  string `json:"kubernetesDistro"`
	KubernetesVersion string `json:"kubernetesVersion"`
	Dockerfile        string `json:"dockerfile"`
	BuildContextDir   string `json:"buildContextDir"`
	OverlayRootfs     string `json:"overlayRootfs"`
	KairosInitImage   string `json:"kairosInitImage"`

	Outputs      artifactOutputs    `json:"outputs"`
	Signing      *signingConfig     `json:"signing,omitempty"`
	Provisioning provisioningConfig `json:"provisioning"`

	CloudConfig string `json:"cloudConfig"`
	OutputDir   string `json:"outputDir"`
}

type artifactOutputs struct {
	ISO         bool `json:"iso"`
	CloudImage  bool `json:"cloudImage"`
	Netboot     bool `json:"netboot"`
	RawDisk     bool `json:"rawDisk"`
	Tar         bool `json:"tar"`
	GCE         bool `json:"gce"`
	VHD         bool `json:"vhd"`
	UKI         bool `json:"uki"`
	FIPS        bool `json:"fips"`
	TrustedBoot bool `json:"trustedBoot"`
}

type signingConfig struct {
	UKIKeySetID        string `json:"ukiKeySetId"`
	UKISecureBootKey   string `json:"ukiSecureBootKey"`
	UKISecureBootCert  string `json:"ukiSecureBootCert"`
	UKITPMPCRKey       string `json:"ukiTpmPcrKey"`
	UKIPublicKeysDir   string `json:"ukiPublicKeysDir"`
	UKISecureBootEnroll string `json:"ukiSecureBootEnroll"`
}

type provisioningConfig struct {
	AutoInstall      *bool  `json:"autoInstall"`
	RegisterAuroraBoot *bool  `json:"registerAuroraBoot"`
	TargetGroupId    string `json:"targetGroupId"`
	UserMode         string `json:"userMode"` // "default", "custom", "none" (defaults to "default")
	Username         string `json:"username"`
	Password         string `json:"password"`
	SSHKeys          string `json:"sshKeys"` // newline-separated public keys
}

// Create handles POST /api/v1/artifacts.
//
//	@Summary		Start a build
//	@Description	Kicks off an asynchronous build. The response is the initial record with phase Pending. Subscribe to /api/v1/ws/ui or poll GET /api/v1/artifacts/{id} to watch it progress through Building then Ready/Error.
//	@Tags			Artifacts
//	@Accept			json
//	@Produce		json
//	@Security		AdminBearer
//	@Param			body	body		APICreateArtifactRequest	true	"Build specification"
//	@Success		201		{object}	store.ArtifactRecord
//	@Failure		400		{object}	APIError
//	@Router			/api/v1/artifacts [post]
func (h *ArtifactHandler) Create(c echo.Context) error {
	var req createArtifactRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	ctx := c.Request().Context()

	// Provisioning defaults: nil means default true.
	autoInstall := true
	if req.Provisioning.AutoInstall != nil {
		autoInstall = *req.Provisioning.AutoInstall
	}
	registerAuroraBoot := true
	if req.Provisioning.RegisterAuroraBoot != nil {
		registerAuroraBoot = *req.Provisioning.RegisterAuroraBoot
	}

	// UKI key set resolution.
	var ukiSBKey, ukiSBCert, ukiTPMKey, ukiPubKeysDir string
	if req.Signing != nil {
		ukiSBKey = req.Signing.UKISecureBootKey
		ukiSBCert = req.Signing.UKISecureBootCert
		ukiTPMKey = req.Signing.UKITPMPCRKey
		ukiPubKeysDir = req.Signing.UKIPublicKeysDir
		if req.Signing.UKIKeySetID != "" && h.secureBootKeys != nil {
			ks, err := h.secureBootKeys.GetByID(ctx, req.Signing.UKIKeySetID)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "key set not found"})
			}
			ukiSBKey = filepath.Join(ks.KeysDir, "db.key")
			ukiSBCert = filepath.Join(ks.KeysDir, "db.pem")
			ukiTPMKey = ks.TPMPCRKeyPath
			ukiPubKeysDir = ks.KeysDir
		}
	}

	// Build opts — set both flat fields and grouped sub-structs.
	opts := builder.BuildOptions{
		ID:                uuid.New().String(),
		Name:              req.Name,
		BaseImage:         req.BaseImage,
		KairosVersion:     req.KairosVersion,
		Model:             req.Model,
		KubernetesDistro:  req.KubernetesDistro,
		KubernetesVersion: req.KubernetesVersion,
		FIPS:              req.Outputs.FIPS,
		TrustedBoot:       req.Outputs.TrustedBoot,
		ISO:               req.Outputs.ISO,
		CloudImage:        req.Outputs.CloudImage,
		Netboot:           req.Outputs.Netboot,
		CloudConfig:       req.CloudConfig,
		OutputDir:         req.OutputDir,
		OverlayRootfs:     req.OverlayRootfs,
		Dockerfile:        req.Dockerfile,
		BuildContextDir:   req.BuildContextDir,
		KairosInitImage:   req.KairosInitImage,
	}
	// Set grouped fields.
	opts.Source = builder.ImageSource{
		BaseImage:         req.BaseImage,
		KairosVersion:     req.KairosVersion,
		Model:             req.Model,
		Arch:              req.Arch,
		Variant:           req.Variant,
		KubernetesDistro:  req.KubernetesDistro,
		KubernetesVersion: req.KubernetesVersion,
	}
	opts.Outputs = builder.OutputOptions{
		ISO:         req.Outputs.ISO,
		CloudImage:  req.Outputs.CloudImage,
		Netboot:     req.Outputs.Netboot,
		RawDisk:     req.Outputs.RawDisk,
		Tar:         req.Outputs.Tar,
		GCE:         req.Outputs.GCE,
		VHD:         req.Outputs.VHD,
		UKI:         req.Outputs.UKI,
		FIPS:        req.Outputs.FIPS,
		TrustedBoot: req.Outputs.TrustedBoot,
	}
	opts.Signing = builder.SigningOptions{
		UKISecureBootKey:  ukiSBKey,
		UKISecureBootCert: ukiSBCert,
		UKITPMPCRKey:      ukiTPMKey,
		UKIPublicKeysDir:  ukiPubKeysDir,
	}
	if req.Signing != nil {
		opts.Signing.UKISecureBootEnroll = req.Signing.UKISecureBootEnroll
	}
	opts.Provisioning = builder.ProvisioningOptions{
		AutoInstall:      autoInstall,
		RegisterAuroraBoot: registerAuroraBoot,
		TargetGroupID:    req.Provisioning.TargetGroupId,
	}

	// Resolve target group name for cloud-config injection.
	groupName := ""
	if req.Provisioning.TargetGroupId != "" && h.groups != nil {
		if g, err := h.groups.GetByID(ctx, req.Provisioning.TargetGroupId); err == nil {
			groupName = g.Name
		}
	}

	// Build the canonical cloud-config from structured provisioning fields.
	// req.CloudConfig is treated as "extra YAML" appended at the end (the
	// Advanced field), NOT a full document — this prevents duplicate top-level
	// keys when the user customizes the advanced section.
	opts.CloudConfig = buildCloudConfig(cloudConfigParams{
		autoInstall:      autoInstall,
		registerAuroraBoot: registerAuroraBoot,
		aurorabootURL:      h.aurorabootURL,
		regToken:         h.regToken,
		groupName:        groupName,
		userMode:         req.Provisioning.UserMode,
		username:         req.Provisioning.Username,
		password:         req.Provisioning.Password,
		sshKeys:          req.Provisioning.SSHKeys,
		extraYAML:        req.CloudConfig,
	})

	status, err := h.builder.Build(ctx, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start build"})
	}

	// Persist the artifact record in the store if available.
	if h.store != nil {
		rec := &store.ArtifactRecord{
			ID:                status.ID,
			Name:              req.Name,
			Phase:             status.Phase,
			Message:           status.Message,
			BaseImage:         req.BaseImage,
			KairosVersion:     req.KairosVersion,
			Model:             req.Model,
			ISO:               req.Outputs.ISO,
			CloudImage:        req.Outputs.CloudImage,
			Netboot:           req.Outputs.Netboot,
			FIPS:              req.Outputs.FIPS,
			TrustedBoot:       req.Outputs.TrustedBoot,
			Arch:              req.Arch,
			Variant:           req.Variant,
			RawDisk:           req.Outputs.RawDisk,
			Tar:               req.Outputs.Tar,
			GCE:               req.Outputs.GCE,
			VHD:               req.Outputs.VHD,
			UKI:               req.Outputs.UKI,
			KairosInitImage:   req.KairosInitImage,
			AutoInstall:       autoInstall,
			RegisterAuroraBoot:  registerAuroraBoot,
			Dockerfile:        req.Dockerfile,
			CloudConfig:       opts.CloudConfig,
			KubernetesDistro:  req.KubernetesDistro,
			KubernetesVersion: req.KubernetesVersion,
			TargetGroupID:     req.Provisioning.TargetGroupId,
			OverlayRootfs:     req.OverlayRootfs,
		}
		_ = h.store.Create(ctx, rec)
	}

	return c.JSON(http.StatusCreated, status)
}

// List handles GET /api/v1/artifacts.
// List handles GET /api/v1/artifacts.
//
//	@Summary	List artifacts
//	@Tags		Artifacts
//	@Produce	json
//	@Security	AdminBearer
//	@Success	200	{array}	store.ArtifactRecord
//	@Router		/api/v1/artifacts [get]
func (h *ArtifactHandler) List(c echo.Context) error {
	if h.store != nil {
		records, err := h.store.List(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list artifacts"})
		}
		if records == nil {
			records = []*store.ArtifactRecord{}
		}
		return c.JSON(http.StatusOK, records)
	}

	// Fall back to builder if no store.
	statuses, err := h.builder.List(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list artifacts"})
	}
	if statuses == nil {
		statuses = []*builder.BuildStatus{}
	}
	return c.JSON(http.StatusOK, statuses)
}

// Get handles GET /api/v1/artifacts/:id.
// Get handles GET /api/v1/artifacts/:id.
//
//	@Summary	Get an artifact
//	@Tags		Artifacts
//	@Produce	json
//	@Security	AdminBearer
//	@Param		id	path		string	true	"Artifact ID"
//	@Success	200	{object}	store.ArtifactRecord
//	@Failure	404	{object}	APIError
//	@Router		/api/v1/artifacts/{id} [get]
func (h *ArtifactHandler) Get(c echo.Context) error {
	id := c.Param("id")

	if h.store != nil {
		rec, err := h.store.GetByID(c.Request().Context(), id)
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found"})
		}
		return c.JSON(http.StatusOK, rec)
	}

	// Fall back to builder if no store.
	status, err := h.builder.Status(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found"})
	}
	return c.JSON(http.StatusOK, status)
}

// GetLogs handles GET /api/v1/artifacts/:id/logs.
// GetLogs handles GET /api/v1/artifacts/:id/logs.
//
//	@Summary		Get build log snapshot
//	@Description	Returns the full concatenated log as text/plain. For real-time updates subscribe to the UI WebSocket and filter {type:"build-log"} envelopes.
//	@Tags			Artifacts
//	@Produce		plain
//	@Security		AdminBearer
//	@Param			id	path		string	true	"Artifact ID"
//	@Success		200	{string}	string
//	@Router			/api/v1/artifacts/{id}/logs [get]
func (h *ArtifactHandler) GetLogs(c echo.Context) error {
	id := c.Param("id")

	if h.store == nil {
		return c.String(http.StatusNotImplemented, "log storage not available")
	}

	logs, err := h.store.GetLogs(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found"})
	}
	return c.String(http.StatusOK, logs)
}

// Cancel handles POST /api/v1/artifacts/:id/cancel.
// Cancel handles POST /api/v1/artifacts/:id/cancel.
//
//	@Summary	Cancel a running build
//	@Tags		Artifacts
//	@Security	AdminBearer
//	@Param		id	path	string	true	"Artifact ID"
//	@Success	200
//	@Router		/api/v1/artifacts/{id}/cancel [post]
func (h *ArtifactHandler) Cancel(c echo.Context) error {
	id := c.Param("id")

	if err := h.builder.Cancel(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found or cannot be cancelled"})
	}

	// Return updated status.
	if h.store != nil {
		rec, err := h.store.GetByID(c.Request().Context(), id)
		if err == nil {
			return c.JSON(http.StatusOK, rec)
		}
	}

	status, err := h.builder.Status(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]string{"status": "cancelled"})
	}
	return c.JSON(http.StatusOK, status)
}

// Download handles GET /api/v1/artifacts/:id/download/*.
func (h *ArtifactHandler) Download(c echo.Context) error {
	id := c.Param("id")
	// Echo uses * for catch-all params; the param name is "*".
	filename := c.Param("*")
	if filename == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "filename required"})
	}

	// Clean the filename to prevent directory traversal.
	filename = filepath.Clean(filename)
	if strings.Contains(filename, "..") {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid filename"})
	}

	// Validate the artifact exists if store is available.
	if h.store != nil {
		_, err := h.store.GetByID(c.Request().Context(), id)
		if err != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found"})
		}
	}

	filePath := filepath.Join(h.artifactsDir, id, filename)

	// Ensure the resolved path is within the artifacts directory.
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid path"})
	}
	absBase, err := filepath.Abs(filepath.Join(h.artifactsDir, id))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid path"})
	}
	if !strings.HasPrefix(absPath, absBase) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid path"})
	}

	return c.File(filePath)
}

// ClearFailed handles DELETE /api/v1/artifacts/failed.
func (h *ArtifactHandler) ClearFailed(c echo.Context) error {
	ctx := c.Request().Context()

	// Clean up output and overlay dirs for failed artifacts before deleting records.
	if records, err := h.store.List(ctx); err == nil {
		for _, r := range records {
			if r.Phase == store.ArtifactError {
				os.RemoveAll(filepath.Join(h.artifactsDir, r.ID))
				if r.OverlayRootfs != "" && strings.HasPrefix(r.OverlayRootfs, h.artifactsDir) {
					os.RemoveAll(r.OverlayRootfs)
				}
			}
		}
	}

	if err := h.store.DeleteByPhase(ctx, store.ArtifactError); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to clear"})
	}
	return c.NoContent(http.StatusNoContent)
}

// Delete handles DELETE /api/v1/artifacts/:id.
// Delete handles DELETE /api/v1/artifacts/:id.
//
//	@Summary	Delete an artifact and its output files
//	@Tags		Artifacts
//	@Security	AdminBearer
//	@Param		id	path	string	true	"Artifact ID"
//	@Success	204
//	@Router		/api/v1/artifacts/{id} [delete]
func (h *ArtifactHandler) Delete(c echo.Context) error {
	id := c.Param("id")
	ctx := c.Request().Context()

	rec, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found"})
	}

	// Cancel if running.
	if rec.Phase == store.ArtifactPending || rec.Phase == store.ArtifactBuilding {
		_ = h.builder.Cancel(ctx, id)
	}

	// Remove build output directory.
	outputDir := filepath.Join(h.artifactsDir, id)
	os.RemoveAll(outputDir)

	// Remove uploaded overlay directory if present.
	if rec.OverlayRootfs != "" && strings.HasPrefix(rec.OverlayRootfs, h.artifactsDir) {
		os.RemoveAll(rec.OverlayRootfs)
	}

	// Remove Docker image.
	if rec.ContainerImage != "" {
		exec.Command("docker", "rmi", "-f", rec.ContainerImage).Run()
	}

	// Delete DB record.
	if err := h.store.Delete(ctx, id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete"})
	}
	return c.NoContent(http.StatusNoContent)
}

// Update handles PATCH /api/v1/artifacts/:id.
// Update handles PATCH /api/v1/artifacts/:id.
//
//	@Summary	Update artifact metadata (name / saved flag)
//	@Tags		Artifacts
//	@Accept		json
//	@Produce	json
//	@Security	AdminBearer
//	@Param		id		path		string						true	"Artifact ID"
//	@Param		body	body		APIUpdateArtifactRequest	true	"Fields to update"
//	@Success	200		{object}	store.ArtifactRecord
//	@Router		/api/v1/artifacts/{id} [patch]
func (h *ArtifactHandler) Update(c echo.Context) error {
	id := c.Param("id")
	ctx := c.Request().Context()

	rec, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found"})
	}

	var patch struct {
		Name  *string `json:"name"`
		Saved *bool   `json:"saved"`
	}
	if err := c.Bind(&patch); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if patch.Name != nil {
		rec.Name = *patch.Name
	}
	if patch.Saved != nil {
		rec.Saved = *patch.Saved
	}
	if err := h.store.Update(ctx, rec); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update"})
	}
	return c.JSON(http.StatusOK, rec)
}

// UploadOverlay handles POST /api/v1/artifacts/upload-overlay.
// Accepts multipart file upload. If the file is .tar.gz/.tgz, extracts it.
// Otherwise saves files directly. Returns the server-side directory path.
func (h *ArtifactHandler) UploadOverlay(c echo.Context) error {
	id := uuid.New().String()
	overlayDir := filepath.Join(h.artifactsDir, "overlays", id)
	if err := os.MkdirAll(overlayDir, 0755); err != nil {
		return c.JSON(500, map[string]string{"error": "failed to create overlay directory"})
	}

	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(400, map[string]string{"error": "invalid multipart form"})
	}

	files := form.File["files"]
	for _, fh := range files {
		src, err := fh.Open()
		if err != nil {
			continue
		}

		name := filepath.Base(fh.Filename)

		// If .tar.gz or .tgz, extract it
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
			cmd := exec.Command("tar", "xzf", "-", "-C", overlayDir)
			cmd.Stdin = src
			if err := cmd.Run(); err != nil {
				src.Close()
				return c.JSON(500, map[string]string{"error": fmt.Sprintf("failed to extract %s: %v", name, err)})
			}
			src.Close()
			continue
		}

		// Otherwise save the file directly
		dst, err := os.Create(filepath.Join(overlayDir, name))
		if err != nil {
			src.Close()
			continue
		}
		io.Copy(dst, src)
		dst.Close()
		src.Close()
	}

	return c.JSON(200, map[string]string{"path": overlayDir})
}

// ExportImage handles GET /api/v1/artifacts/:id/image.
// Flattens multi-layer images via docker export + import + save to avoid
// symlink ordering issues across OCI layers (e.g. /boot/vmlinuz).
func (h *ArtifactHandler) ExportImage(c echo.Context) error {
	id := c.Param("id")
	ctx := c.Request().Context()

	rec, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found"})
	}
	if rec.ContainerImage == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no container image"})
	}

	// Flatten the image to a single layer to avoid symlink ordering issues.
	// Multi-layer OCI images can have conflicting symlinks across layers
	// (e.g. /boot/vmlinuz pointing to wrong target in earlier layer).
	// Pipeline: docker create → docker export (flat tar) → docker import (single-layer image) → docker save (OCI tar)
	flatImage := fmt.Sprintf("auroraboot-flat:%s", id)
	cid := fmt.Sprintf("auroraboot-export-%s", id)

	// Create container and export flat tar, pipe into docker import
	createCmd := exec.CommandContext(ctx, "docker", "create", "--name", cid, rec.ContainerImage, "true")
	if err := createCmd.Run(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("creating container for export: %v", err)})
	}
	defer exec.Command("docker", "rm", cid).Run()

	// Export flat tar and import as single-layer image
	exportCmd := exec.CommandContext(ctx, "docker", "export", cid)
	importCmd := exec.CommandContext(ctx, "docker", "import", "-", flatImage)
	importCmd.Stdin, err = exportCmd.StdoutPipe()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "pipe setup failed"})
	}

	if err := importCmd.Start(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "docker import start failed"})
	}
	if err := exportCmd.Run(); err != nil {
		importCmd.Process.Kill()
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("docker export failed: %v", err)})
	}
	if err := importCmd.Wait(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("docker import failed: %v", err)})
	}
	defer exec.Command("docker", "rmi", flatImage).Run()

	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.tar", id))

	// Save the single-layer image as proper OCI tar
	cmd := exec.CommandContext(ctx, "docker", "save", flatImage)
	cmd.Stdout = c.Response().Writer
	return cmd.Run()
}

// cloudConfigParams collects the inputs needed to assemble a node's cloud-config.
type cloudConfigParams struct {
	autoInstall      bool
	registerAuroraBoot bool
	aurorabootURL      string
	regToken         string
	groupName        string
	userMode         string // "default", "custom", "none"
	username         string
	password         string
	sshKeys          string // newline-separated public keys
	extraYAML        string // optional: appended verbatim after the canonical block
}

// buildCloudConfig assembles a Kairos cloud-config YAML document from structured
// provisioning fields. It's the single source of truth — the frontend never builds
// its own document, only sends the structured fields plus optional extra YAML.
//
// The result has at most one top-level key for each of install/phonehome/stages,
// avoiding the duplicate-key problem that arose when frontend and user input both
// emitted overlapping sections. When the extra YAML provides its own stages block
// (e.g. boot, after-install), it is merged under the canonical stages key rather
// than appended as a second top-level entry.
func buildCloudConfig(p cloudConfigParams) string {
	doc := map[string]interface{}{}

	if p.autoInstall {
		doc["install"] = map[string]interface{}{
			"auto":   true,
			"device": "auto",
			"reboot": true,
		}
	}

	if p.registerAuroraBoot {
		doc["phonehome"] = map[string]interface{}{
			"url":                p.aurorabootURL,
			"registration_token": p.regToken,
			"group":              p.groupName,
		}
	}

	// Build the user block under stages.<stage>.
	mode := p.userMode
	if mode == "" {
		mode = "default"
	}
	if mode != "none" {
		username := p.username
		password := p.password
		if mode == "default" || username == "" {
			username = "kairos"
		}
		if mode == "default" || password == "" {
			password = "kairos"
		}

		sshLines := []string{}
		for _, line := range strings.Split(p.sshKeys, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				sshLines = append(sshLines, line)
			}
		}
		stage := "initramfs"
		if len(sshLines) > 0 {
			stage = "network"
		}

		userEntry := map[string]interface{}{
			"passwd": password,
			"groups": []interface{}{"admin"},
		}
		if len(sshLines) > 0 {
			keys := make([]interface{}, len(sshLines))
			for i, k := range sshLines {
				keys[i] = k
			}
			userEntry["ssh_authorized_keys"] = keys
		}
		doc["stages"] = map[string]interface{}{
			stage: []interface{}{
				map[string]interface{}{
					"users": map[string]interface{}{
						username: userEntry,
					},
				},
			},
		}
	}

	// Merge extra YAML (the Advanced field) on top of the canonical doc.
	// If the user provided their own stages.boot or install: section, it gets
	// merged under the corresponding top-level key instead of producing a
	// duplicate top-level key.
	extra := strings.TrimSpace(p.extraYAML)
	if extra != "" {
		extra = strings.TrimPrefix(extra, "#cloud-config")
		extra = strings.TrimLeft(extra, "\n\r ")
		var extraDoc map[string]interface{}
		if err := yaml.Unmarshal([]byte(extra), &extraDoc); err == nil && extraDoc != nil {
			mergeYAML(doc, extraDoc)
		}
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		// Fall back to a minimal config rather than failing the build.
		return "#cloud-config\n"
	}
	return "#cloud-config\n" + string(out)
}

// mergeYAML recursively merges src into dst. Maps are merged key-by-key; slices
// from src are appended to slices in dst with the same key; scalar conflicts are
// resolved in favor of src (user-provided extra wins over canonical defaults).
func mergeYAML(dst, src map[string]interface{}) {
	for k, sv := range src {
		dv, exists := dst[k]
		if !exists {
			dst[k] = sv
			continue
		}
		// Both maps → merge recursively.
		if dvMap, ok := dv.(map[string]interface{}); ok {
			if svMap, ok := sv.(map[string]interface{}); ok {
				mergeYAML(dvMap, svMap)
				continue
			}
		}
		// Both slices → concatenate.
		if dvSlice, ok := dv.([]interface{}); ok {
			if svSlice, ok := sv.([]interface{}); ok {
				dst[k] = append(dvSlice, svSlice...)
				continue
			}
		}
		// Otherwise, the extra YAML overrides.
		dst[k] = sv
	}
}
