package handlers

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"
)

// uploadTokenBytes is the raw entropy for a per-build UploadToken; 32 random
// bytes hex-encoded gives a 64-character bearer with 256 bits of entropy.
const uploadTokenBytes = 32

// maxUploadBytes caps a single exporter upload. 20 GiB covers any single
// Kairos artifact today (an oversized UKI ISO is a few GiB); the limit exists
// to keep a runaway exporter from filling the AuroraBoot host's disk.
const maxUploadBytes = 20 * 1024 * 1024 * 1024

func mintUploadToken() (string, error) {
	b := make([]byte, uploadTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("mint upload token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// hashUploadToken is the digest AuroraBoot stores in place of the plaintext
// upload token. Plaintext only lives in the per-build k8s Secret the
// exporter reads; the DB never sees it. Upload hashes the presented bearer
// with the same function and constant-time compares against the stored
// digest. sha256 is sufficient here because the token is 32 bytes of
// crypto/rand output - no rainbow table applies and no work factor is
// needed to slow guessing.
func hashUploadToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// ArtifactHandler handles artifact-related REST endpoints.
type ArtifactHandler struct {
	builder        builder.ArtifactBuilder
	store          store.ArtifactStore
	groups         store.GroupStore
	secureBootKeys store.SecureBootKeySetStore
	extensions     store.ExtensionStore
	bundles        store.ArtifactExtensionBundleStore
	regToken       string
	aurorabootURL  string
	artifactsDir   string
}

// NewArtifactHandler creates a new ArtifactHandler.
func NewArtifactHandler(
	b builder.ArtifactBuilder,
	artifactStore store.ArtifactStore,
	groups store.GroupStore,
	secureBootKeys store.SecureBootKeySetStore,
	extensions store.ExtensionStore,
	bundles store.ArtifactExtensionBundleStore,
	artifactsDir string,
	regToken string,
	aurorabootURL string,
) *ArtifactHandler {
	return &ArtifactHandler{
		builder:        b,
		store:          artifactStore,
		groups:         groups,
		secureBootKeys: secureBootKeys,
		extensions:     extensions,
		bundles:        bundles,
		regToken:       regToken,
		aurorabootURL:  aurorabootURL,
		artifactsDir:   artifactsDir,
	}
}

// createArtifactRequest is the expected body for creating an artifact build.
type createArtifactRequest struct {
	Name                    string   `json:"name"`
	BaseImage               string   `json:"baseImage"`
	KairosVersion           string   `json:"kairosVersion"`
	Model                   string   `json:"model"`
	Arch                    string   `json:"arch"`
	Variant                 string   `json:"variant"`
	KubernetesDistro        string   `json:"kubernetesDistro"`
	KubernetesVersion       string   `json:"kubernetesVersion"`
	KubernetesEnabled       *bool    `json:"kubernetesEnabled"`
	AllowInsecureRegistries bool     `json:"allow-insecure-registries"`
	Dockerfile              string   `json:"dockerfile"`
	HadronBase              string   `json:"hadronBase"`
	HadronFirmware          []string `json:"hadronFirmware"`
	HadronLayers            []string `json:"hadronLayers"`
	HadronExtra             string   `json:"hadronExtra"`
	BuildContextDir         string   `json:"buildContextDir"`
	OverlayRootfs           string   `json:"overlayRootfs"`
	KairosInitImage         string   `json:"kairosInitImage"`

	Outputs      artifactOutputs    `json:"outputs"`
	Signing      *signingConfig     `json:"signing,omitempty"`
	Provisioning provisioningConfig `json:"provisioning"`

	CloudConfig string `json:"cloudConfig"`
	OutputDir   string `json:"outputDir"`

	ExtensionHierarchies *extensionHierarchiesReq `json:"extensionHierarchies,omitempty"`
	BundledExtensions    []createBundleEntry      `json:"bundledExtensions,omitempty"`
}

type extensionHierarchiesReq struct {
	Sysext  []string `json:"sysext"`
	Confext []string `json:"confext"`
}

type createBundleEntry struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	PinnedVersion string `json:"pinnedVersion,omitempty"`
	Order         int    `json:"order,omitempty"`
}

type artifactOutputs struct {
	ISO         bool `json:"iso"`
	CloudImage  bool `json:"cloudImage"`
	Netboot     bool `json:"netboot"`
	RawDisk     bool `json:"rawDisk"`
	Tar         bool `json:"tar"`
	GCE         bool `json:"gce"`
	VHD         bool `json:"vhd"`
	MAAS        bool `json:"maas"`
	UKI         bool `json:"uki"`
	FIPS        bool `json:"fips"`
	TrustedBoot bool `json:"trustedBoot"`
}

type signingConfig struct {
	UKIKeySetID         string `json:"ukiKeySetId"`
	UKISecureBootKey    string `json:"ukiSecureBootKey"`
	UKISecureBootCert   string `json:"ukiSecureBootCert"`
	UKITPMPCRKey        string `json:"ukiTpmPcrKey"`
	UKIPublicKeysDir    string `json:"ukiPublicKeysDir"`
	UKISecureBootEnroll string `json:"ukiSecureBootEnroll"`
}

type provisioningConfig struct {
	AutoInstall        *bool    `json:"autoInstall"`
	RegisterAuroraBoot *bool    `json:"registerAuroraBoot"`
	TargetGroupId      string   `json:"targetGroupId"`
	UserMode           string   `json:"userMode"` // "default", "custom", "none" (defaults to "default")
	Username           string   `json:"username"`
	Password           string   `json:"password"`
	SSHKeys            string   `json:"sshKeys"` // newline-separated public keys
	AllowedCommands    []string `json:"allowedCommands"`
}

// phonehomeSafeDefaults is the conservative set of commands AuroraBoot bakes
// into cloud-configs when the operator has not specified a custom selection.
// This list must stay aligned with kairos-agent's DefaultAllowedCommands so
// the UX's "default safe set" label corresponds to what the agent actually
// permits if the emitted list were ever absent.
// `unregister` is in the safe defaults so the Decommission flow works
// out of the box. It's a self-destruct of the management link, not a
// privilege escalation — a rogue server can only ever terminate its own
// connection. Operators who want to disable remote decommission can
// untick it in the UI; they'll then have to SSH in and run
// `kairos-agent phone-home uninstall` to tear down a node by hand.
var phonehomeSafeDefaults = []string{"upgrade", "upgrade-recovery", "reboot", "unregister"}

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

	// Hierarchies validation: sysext list cannot include /usr or /; confext list
	// cannot include /etc or /. validateHierarchies (in extensions.go) covers /
	// and /usr generically; the /etc rule is inline below for the confext branch.
	var sysHierarchies, conHierarchies []string
	if req.ExtensionHierarchies != nil {
		var verr error
		sysHierarchies, verr = validateHierarchies(req.ExtensionHierarchies.Sysext)
		if verr != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "sysext " + verr.Error()})
		}
		for i, p := range req.ExtensionHierarchies.Confext {
			p = strings.TrimRight(p, "/")
			if p == "/etc" || p == "/" {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": fmt.Sprintf("confext hierarchies[%d]: %q is implicit and cannot be listed", i, p),
				})
			}
		}
		conHierarchies, verr = validateHierarchies(req.ExtensionHierarchies.Confext)
		if verr != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "confext " + verr.Error()})
		}
	}

	// bundledExtensions: validate each entry resolves to a Ready extension of
	// the matching arch. ArtifactID is filled in after the artifact record is
	// persisted (we don't know the ID until then).
	bundleRows := make([]store.ArtifactExtensionBundle, 0, len(req.BundledExtensions))
	if len(req.BundledExtensions) > 0 {
		if h.extensions == nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "extensions store not configured"})
		}
		for i, b := range req.BundledExtensions {
			if b.Name == "" {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("bundledExtensions[%d]: name required", i)})
			}
			if b.Type != "sysext" && b.Type != "confext" {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("bundledExtensions[%d]: type must be sysext or confext", i)})
			}
			var ext *store.ExtensionRecord
			var rErr error
			if b.PinnedVersion != "" {
				ext, rErr = h.extensions.FindByNameAndVersion(ctx, b.Type, b.Name, b.PinnedVersion)
			} else {
				ext, rErr = h.extensions.FindLatestReadyByName(ctx, b.Type, b.Name)
			}
			if rErr != nil || ext == nil {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": fmt.Sprintf("bundledExtensions[%d]: no Ready %s extension matches name=%q version=%q",
						i, b.Type, b.Name, b.PinnedVersion),
				})
			}
			if ext.Arch != req.Arch {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": fmt.Sprintf("bundledExtensions[%d]: arch %q != artifact arch %q",
						i, ext.Arch, req.Arch),
				})
			}
			bundleRows = append(bundleRows, store.ArtifactExtensionBundle{
				ExtensionName: b.Name,
				ExtensionType: b.Type,
				PinnedVersion: b.PinnedVersion,
				Order:         b.Order,
			})
		}
	}

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

	// Mint the per-build upload token before we hand opts to the builder so
	// the operator backend's exporter Secret carries a fresh token for every
	// build, and the store record can validate the incoming PUT /upload.
	uploadToken, err := mintUploadToken()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to prepare build"})
	}

	// Build opts — set both flat fields and grouped sub-structs.
	opts := builder.BuildOptions{
		ID:                uuid.New().String(),
		Name:              req.Name,
		UploadToken:       uploadToken,
		LogRedactValues:   []string{h.regToken, req.Provisioning.Password},
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
		HadronBase:        req.HadronBase,
		HadronFirmware:    req.HadronFirmware,
		HadronLayers:      req.HadronLayers,
		HadronExtra:       req.HadronExtra,
	}
	// Set grouped fields.
	opts.Source = builder.ImageSource{
		BaseImage:               req.BaseImage,
		KairosVersion:           req.KairosVersion,
		Model:                   req.Model,
		Arch:                    req.Arch,
		Variant:                 req.Variant,
		KubernetesDistro:        req.KubernetesDistro,
		KubernetesVersion:       req.KubernetesVersion,
		AllowInsecureRegistries: req.AllowInsecureRegistries,
	}
	opts.Outputs = builder.OutputOptions{
		ISO:         req.Outputs.ISO,
		CloudImage:  req.Outputs.CloudImage,
		Netboot:     req.Outputs.Netboot,
		RawDisk:     req.Outputs.RawDisk,
		Tar:         req.Outputs.Tar,
		GCE:         req.Outputs.GCE,
		VHD:         req.Outputs.VHD,
		MAAS:        req.Outputs.MAAS,
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
	// Substitute the safe defaults when the client omits allowedCommands, so
	// the generated cloud-config always carries an explicit phonehome.allowed_commands.
	// An empty-but-non-nil slice is preserved verbatim (observe-only mode).
	allowedCommands := req.Provisioning.AllowedCommands
	if allowedCommands == nil {
		allowedCommands = append([]string(nil), phonehomeSafeDefaults...)
	}

	kubernetesEnabled := true
	if req.KubernetesEnabled != nil {
		kubernetesEnabled = *req.KubernetesEnabled
	}

	opts.Provisioning = builder.ProvisioningOptions{
		AutoInstall:        autoInstall,
		RegisterAuroraBoot: registerAuroraBoot,
		TargetGroupID:      req.Provisioning.TargetGroupId,
		AllowedCommands:    allowedCommands,
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
		autoInstall:        autoInstall,
		registerAuroraBoot: registerAuroraBoot,
		aurorabootURL:      h.aurorabootURL,
		regToken:           h.regToken,
		groupName:          groupName,
		allowedCommands:    allowedCommands,
		variant:            req.Variant,
		kubernetesDistro:   req.KubernetesDistro,
		kubernetesEnabled:  kubernetesEnabled,
		userMode:           req.Provisioning.UserMode,
		username:           req.Provisioning.Username,
		password:           req.Provisioning.Password,
		sshKeys:            req.Provisioning.SSHKeys,
		extraYAML:          req.CloudConfig,
		sysextHierarchies:  sysHierarchies,
		confextHierarchies: conHierarchies,
	})

	status, err := h.builder.Build(ctx, opts)
	if err != nil {
		// Invalid admin-supplied build inputs are a client error (400), not a
		// server fault (500). The validation detail (field + "invalid") is safe
		// to surface; it carries no secrets.
		if errors.Is(err, builder.ErrInvalidBuildOptions) {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		// A builder that cannot service this request (e.g. a scaffolded backend
		// or one that does not produce a requested output) is 501, not 500.
		if errors.Is(err, builder.ErrNotSupported) {
			return c.JSON(http.StatusNotImplemented, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to start build"})
	}

	// Persist the artifact record in the store if available.
	//
	// The production builder also persists the record (keyed by status.ID)
	// from inside Build(); when both run, this Create no-ops on the duplicate
	// primary key. But builders that don't persist (e.g. the mock builder used
	// in tests, or any builder constructed without a store) rely on this Create
	// being the one that writes the row — so keep it. Both sites set the same
	// fields, including AllowInsecureRegistries, so the stored record is identical either way.
	if h.store != nil {
		rec := &store.ArtifactRecord{
			ID:                      status.ID,
			Name:                    req.Name,
			UploadToken:             hashUploadToken(uploadToken),
			Phase:                   status.Phase,
			Message:                 status.Message,
			BaseImage:               req.BaseImage,
			KairosVersion:           req.KairosVersion,
			Model:                   req.Model,
			ISO:                     req.Outputs.ISO,
			CloudImage:              req.Outputs.CloudImage,
			Netboot:                 req.Outputs.Netboot,
			FIPS:                    req.Outputs.FIPS,
			TrustedBoot:             req.Outputs.TrustedBoot,
			Arch:                    req.Arch,
			Variant:                 req.Variant,
			AllowInsecureRegistries: req.AllowInsecureRegistries,
			RawDisk:                 req.Outputs.RawDisk,
			Tar:                     req.Outputs.Tar,
			GCE:                     req.Outputs.GCE,
			VHD:                     req.Outputs.VHD,
			MAAS:                    req.Outputs.MAAS,
			UKI:                     req.Outputs.UKI,
			KairosInitImage:         req.KairosInitImage,
			AutoInstall:             autoInstall,
			RegisterAuroraBoot:      registerAuroraBoot,
			Dockerfile:              req.Dockerfile,
			HadronBase:              req.HadronBase,
			HadronFirmware:          req.HadronFirmware,
			HadronLayers:            req.HadronLayers,
			HadronExtra:             req.HadronExtra,
			CloudConfig:             opts.CloudConfig,
			KubernetesDistro:        req.KubernetesDistro,
			KubernetesVersion:       req.KubernetesVersion,
			KubernetesEnabled:       boolPtr(kubernetesEnabled),
			TargetGroupID:           req.Provisioning.TargetGroupId,
			OverlayRootfs:           req.OverlayRootfs,
			ExtensionHierarchies: store.ExtensionHierarchies{
				Sysext:  sysHierarchies,
				Confext: conHierarchies,
			},
		}
		// The builder.Build above synchronously creates a minimal record
		// (internal/builder/auroraboot/builder.go:203) before kicking off
		// the goroutine. That Create wins the unique-id race; this handler's
		// Create silently dups and the user-facing fields here (notably
		// ExtensionHierarchies) get lost. Update brings the handler's view
		// in alongside the builder's. The status fields (Phase/Message) are
		// the builder's responsibility from this point on, so we leave them
		// to the goroutine to overwrite via Update later.
		if err := h.store.Update(ctx, rec); err != nil {
			c.Logger().Errorf("persist artifact handler-side fields for %s: %v", status.ID, err)
		}
		// A builder that persists on its own (the local backend) will have
		// already written the row before Build returned; a builder that does
		// not (the operator backend, and the mock builder used in tests)
		// relies on this Create being the one that writes it. Check first
		// so a legitimate Create failure for the operator path is not
		// masked as a benign duplicate-primary-key error.
		if _, err := h.store.GetByID(ctx, status.ID); err != nil {
			if err := h.store.Create(ctx, rec); err != nil {
				// The operator backend has already Created an OSArtifact
				// CR at this point. Failing to persist the row would leave
				// the cluster with a live CR the DB knows nothing about;
				// the UI cannot List/Get/Cancel/Delete it. Reap it
				// best-effort so a retry lands in a clean state, then
				// surface the failure.
				if cancelErr := h.builder.Cancel(ctx, status.ID); cancelErr != nil && !errors.Is(cancelErr, builder.ErrNotSupported) {
					fmt.Fprintf(os.Stderr, "create: reap phantom CR %q after store.Create failed: %v\n", status.ID, cancelErr)
				}
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to persist build"})
			}
		}
	}

	// Persist bundle rows once the artifact has an ID. Errors here are
	// non-fatal: the operator can re-attach via PUT /bundle-extensions.
	if h.bundles != nil && len(bundleRows) > 0 {
		for i := range bundleRows {
			bundleRows[i].ArtifactID = status.ID
		}
		if err := h.bundles.ReplaceForArtifact(ctx, status.ID, bundleRows); err != nil {
			c.Logger().Errorf("persist bundle for %s: %v", status.ID, err)
		}
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
		if errors.Is(err, builder.ErrNotSupported) {
			return c.JSON(http.StatusNotImplemented, map[string]string{"error": err.Error()})
		}
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
		if errors.Is(err, builder.ErrNotSupported) {
			return c.JSON(http.StatusNotImplemented, map[string]string{"error": err.Error()})
		}
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

// Upload handles PUT /api/v1/artifacts/:id/upload/*.
//
// The operator backend's exporter Job hits this endpoint once per artifact
// file to ship a finished build back to AuroraBoot's on-disk store, so that
// Download can serve it the same way whether the build was produced locally
// or delegated to an in-cluster operator. Authenticated by the per-build
// bearer minted in Create and stored on the ArtifactRecord.
//
//	@Summary	Upload a single artifact file for a build
//	@Description	Per-build endpoint the operator backend's exporter uses to ship finished artifacts back to AuroraBoot. Bearer is the UploadToken minted at Create time; the admin bearer does not grant access.
//	@Tags		Artifacts
//	@Accept		octet-stream
//	@Param		id			path	string	true	"Artifact ID"
//	@Param		filename	path	string	true	"Artifact filename (single-segment; no path separators)"
//	@Success	201
//	@Failure	400	{object}	APIError
//	@Failure	401	{object}	APIError
//	@Failure	404	{object}	APIError
//	@Failure	413	{object}	APIError
//	@Router		/api/v1/artifacts/{id}/upload/{filename} [put]
func (h *ArtifactHandler) Upload(c echo.Context) error {
	id := c.Param("id")
	filename := c.Param("*")
	ctx := c.Request().Context()

	if err := safePathSegment(id); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid artifact id"})
	}
	if err := safePathSegment(filename); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid filename"})
	}
	clean := filename

	if h.store == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found"})
	}
	rec, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found"})
	}

	presented := strings.TrimPrefix(c.Request().Header.Get("Authorization"), "Bearer ")
	// rec.UploadToken holds a sha256 hex digest, not the plaintext. Hash the
	// presented bearer and constant-time compare so an attacker with DB read
	// access (backup, SQLite file, SQLi elsewhere) never sees a live token.
	if presented == "" || rec.UploadToken == "" ||
		subtle.ConstantTimeCompare([]byte(hashUploadToken(presented)), []byte(rec.UploadToken)) != 1 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid upload token"})
	}

	// Once a build has reached a terminal phase the artifacts it advertises
	// are consumed by nodes and BMCs; a late upload with a still-valid token
	// (leaked or replayed) would overwrite them out from under those
	// consumers. watchCRPhase clears the token on the terminal transition,
	// so a legitimate exporter Job that finishes before its own build is
	// marked Ready still lands; anything after that fails here or fails the
	// token check above.
	if rec.Phase == store.ArtifactReady || rec.Phase == store.ArtifactError {
		return c.JSON(http.StatusConflict, map[string]string{"error": "build is not accepting uploads"})
	}

	buildDir := filepath.Join(h.artifactsDir, id)
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to prepare artifact directory"})
	}

	// Stream to a sibling temp file and rename in place so a partial upload
	// never appears under the destination path. maxUploadBytes caps the body
	// so a runaway exporter cannot fill the AuroraBoot host's disk.
	tmp, err := os.CreateTemp(buildDir, ".upload-*")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open upload buffer"})
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	limited := http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxUploadBytes)
	if _, err := io.Copy(tmp, limited); err != nil {
		_ = tmp.Close()
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return c.JSON(http.StatusRequestEntityTooLarge, map[string]string{"error": "upload exceeds size limit"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "upload write failed"})
	}
	if err := tmp.Close(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "upload close failed"})
	}

	dst := filepath.Join(buildDir, clean)
	if err := os.Rename(tmpPath, dst); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "upload finalize failed"})
	}

	// Reflect the newly-uploaded file in the store record so the UI's
	// existing artifact list (which reads ArtifactFiles verbatim) surfaces
	// operator-produced artifacts the same way it does local ones. Dedupe
	// the append so a retried exporter Job (backoffLimit) does not stack
	// duplicates. UpdateFiles writes only the artifact_files column so a
	// concurrent watchCRPhase transition cannot be clobbered by rec's
	// snapshotted phase/message. Failures are non-fatal - the file is on
	// disk, the store row can be reconciled later, and returning 500 here
	// would mislead the exporter into retrying an already-durable upload.
	appendArtifactFile(rec, clean)
	_ = h.store.UpdateFiles(ctx, id, rec.ArtifactFiles)

	return c.NoContent(http.StatusCreated)
}

// appendArtifactFile adds name to rec.ArtifactFiles if it is not already
// present. Kept as a small helper so tests can reason about it in isolation
// from the Upload handler.
func appendArtifactFile(rec *store.ArtifactRecord, name string) {
	for _, existing := range rec.ArtifactFiles {
		if existing == name {
			return
		}
	}
	rec.ArtifactFiles = append(rec.ArtifactFiles, name)
}

// safePathSegment rejects a URL path parameter that cannot be safely used as
// a single filesystem path segment. Both Upload and Download route their id
// and filename inputs through here before touching the store or the disk:
// once at the boundary we can trust the value downstream. "." by itself is
// not a traversal but is meaningless as an id or filename, so it is rejected
// too rather than resolving to the parent directory later.
func safePathSegment(s string) error {
	if s == "" || s == "." {
		return errors.New("empty segment")
	}
	if strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") || filepath.IsAbs(s) {
		return errors.New("path traversal")
	}
	return nil
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
	ctx := c.Request().Context()

	// Always drive builder.Cancel first so backend state gets reclaimed even
	// when the store lookup is racing a restart or a transient DB failure.
	// Cancel is idempotent on both backends (local: unknown id -> nil;
	// operator: NotFound -> nil), so a stale invocation is safe.
	cancelErr := h.builder.Cancel(ctx, id)
	if errors.Is(cancelErr, builder.ErrNotSupported) {
		return c.JSON(http.StatusNotImplemented, map[string]string{"error": cancelErr.Error()})
	}

	// Consult the store for the response body. Response matrix:
	//   cancel OK + record   -> 200 with record
	//   cancel OK + no store -> 200 {"status":"cancelled"}
	//   cancel FAIL + record -> 500 (record still there; backend refused)
	//   cancel FAIL + no rec -> 404 (unknown id and backend refused too)
	if h.store == nil {
		if cancelErr != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found or cannot be cancelled"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "cancelled"})
	}

	rec, err := h.store.GetByID(ctx, id)
	if err != nil {
		// The store lookup failed (missing row or transient DB error). If
		// the builder also refused, treat this as a 404 to match the old
		// UX for orphaned rows; a hard 500 for a DB blip would train the
		// UI to retry cancels indefinitely.
		if cancelErr != nil {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "artifact not found or cannot be cancelled"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "cancelled"})
	}
	if cancelErr != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "cancel failed"})
	}
	return c.JSON(http.StatusOK, rec)
}

// Download handles GET /api/v1/artifacts/:id/download/*.
func (h *ArtifactHandler) Download(c echo.Context) error {
	id := c.Param("id")
	// Echo uses * for catch-all params; the param name is "*".
	filename := c.Param("*")

	if err := safePathSegment(id); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid artifact id"})
	}
	if err := safePathSegment(filename); err != nil {
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

	// Clean up output and overlay dirs for failed artifacts before deleting
	// records. Forward a Cancel to the builder for each so terminal-phase
	// OSArtifact CRs (operator backend) and their owned PVCs leave the
	// cluster alongside the DB row. Cancel is best-effort: a backend error
	// here should not block DB cleanup, since the row is what the UI keys
	// off, and Cancel is idempotent on both backends.
	if records, err := h.store.List(ctx); err == nil {
		for _, r := range records {
			if r.Phase == store.ArtifactError {
				if cancelErr := h.builder.Cancel(ctx, r.ID); cancelErr != nil && !errors.Is(cancelErr, builder.ErrNotSupported) {
					fmt.Fprintf(os.Stderr, "clear-failed: builder.Cancel(%q) failed: %v\n", r.ID, cancelErr)
				}
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

	// Cancel is best-effort AND unconditional. Delete's job is to reclaim
	// state, and refusing here would strand the row and its on-disk files
	// whenever the backend cannot cancel (a scaffold that returns
	// ErrNotSupported for every id cannot tell "not mine" from "cannot
	// cancel", so refusing would trap orphaned rows forever). We forward the
	// call for every phase, not just Pending/Building, because the operator
	// backend also owns state for terminal-phase records (the OSArtifact CR
	// and its PVC); those need reaping too. The subsequent os.RemoveAll and
	// store.Delete complete the cleanup regardless.
	if cancelErr := h.builder.Cancel(ctx, id); cancelErr != nil && !errors.Is(cancelErr, builder.ErrNotSupported) {
		fmt.Fprintf(os.Stderr, "delete: builder.Cancel(%q) failed: %v\n", id, cancelErr)
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

		// If .tar.gz or .tgz, extract it with full path containment.
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
			if err := extractOverlayTarGz(src, overlayDir); err != nil {
				src.Close()
				return c.JSON(400, map[string]string{"error": fmt.Sprintf("failed to extract %s: %v", name, err)})
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

// maxOverlaySize caps the total uncompressed bytes extracted from a single
// uploaded overlay archive, closing a zip-bomb avenue. Overlays carry config
// fragments and small assets, so 512 MiB is generous headroom.
const maxOverlaySize = 512 * 1024 * 1024

// maxOverlayFileSize caps a single member's uncompressed size within an
// overlay archive.
const maxOverlayFileSize = 256 * 1024 * 1024

// extractOverlayTarGz extracts a gzipped tar stream into destDir with strict
// hygiene, mirroring the SecureBoot ImportKeys hardening: it rejects absolute
// paths and parent-dir escapes, refuses symlinks and hardlinks, and enforces a
// total and per-file size cap. Only regular files and directories are written.
func extractOverlayTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("opening gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()

	// Resolve the destination once so we can confirm every member stays under it.
	cleanDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("resolving overlay directory: %w", err)
	}

	tr := tar.NewReader(gz)
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar entry: %w", err)
		}

		// Reject symlinks and hardlinks outright — they are an escape vector.
		switch hdr.Typeflag {
		case tar.TypeReg, tar.TypeDir:
			// allowed
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("archive contains a link entry %q which is not allowed", hdr.Name)
		default:
			// Skip device/fifo/etc. entries silently.
			continue
		}

		// Path hygiene: no absolute paths, no parent-dir escapes, and the
		// resolved target must stay strictly under destDir.
		cleaned := filepath.Clean(hdr.Name)
		if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %q", hdr.Name)
		}
		target := filepath.Join(cleanDest, cleaned)
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe path in archive: %q escapes overlay directory", hdr.Name)
		}

		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("creating directory %q: %w", hdr.Name, err)
			}
			continue
		}

		if hdr.Size > maxOverlayFileSize {
			return fmt.Errorf("file %q exceeds per-file size limit", hdr.Name)
		}
		total += hdr.Size
		if total > maxOverlaySize {
			return fmt.Errorf("archive exceeds total size limit")
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating parent directory for %q: %w", hdr.Name, err)
		}
		mode := os.FileMode(hdr.Mode).Perm()
		if mode == 0 {
			mode = 0o644
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			return fmt.Errorf("creating file %q: %w", hdr.Name, err)
		}
		// Bound the copy with a hard limit so a lying header can't overrun the cap.
		if _, err := io.CopyN(dst, tr, hdr.Size+1); err != nil && !errors.Is(err, io.EOF) {
			_ = dst.Close()
			return fmt.Errorf("writing file %q: %w", hdr.Name, err)
		}
		if err := dst.Close(); err != nil {
			return fmt.Errorf("closing file %q: %w", hdr.Name, err)
		}
	}
	return nil
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
	autoInstall        bool
	registerAuroraBoot bool
	aurorabootURL      string
	regToken           string
	groupName          string
	// allowedCommands is always emitted verbatim when registerAuroraBoot is true.
	// Callers substitute phonehomeSafeDefaults for nil input before calling.
	allowedCommands   []string
	variant           string // "core" or "standard"
	kubernetesDistro  string // "k3s" or "k0s" when variant=standard
	kubernetesEnabled bool   // cloud-config k3s/k0s.enabled (standard variant only)
	userMode          string // "default", "custom", "none"
	username          string
	password          string
	sshKeys           string // newline-separated public keys
	extraYAML         string // optional: appended verbatim after the canonical block

	// Extension hierarchies declared by the operator. When non-empty, a systemd
	// drop-in is written under stages.initramfs.files so the OS image boots
	// with SYSTEMD_{SYSEXT,CONFEXT}_HIERARCHIES extended beyond their defaults.
	sysextHierarchies  []string
	confextHierarchies []string
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

	if p.variant == "standard" && p.kubernetesDistro != "" {
		switch p.kubernetesDistro {
		case "k3s":
			doc["k3s"] = map[string]interface{}{"enabled": p.kubernetesEnabled}
		case "k0s":
			doc["k0s"] = map[string]interface{}{"enabled": p.kubernetesEnabled}
		}
	}

	if p.registerAuroraBoot {
		// allowed_commands is *always* emitted: AuroraBoot is the operator-facing
		// surface and the list is an explicit decision, not an agent-side default.
		// Callers that pass nil must have already substituted phonehomeSafeDefaults.
		allowed := p.allowedCommands
		if allowed == nil {
			allowed = phonehomeSafeDefaults
		}
		// Convert to []interface{} so yaml.v3 emits a proper sequence even when
		// allowed is empty (-> `[]` rather than a missing key).
		allowedYAML := make([]interface{}, len(allowed))
		for i, c := range allowed {
			allowedYAML[i] = c
		}
		doc["phonehome"] = map[string]interface{}{
			"url":                p.aurorabootURL,
			"registration_token": p.regToken,
			"group":              p.groupName,
			"allowed_commands":   allowedYAML,
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

	// Extension hierarchies: write a systemd drop-in under stages.initramfs so
	// the OS image boots with SYSTEMD_{SYSEXT,CONFEXT}_HIERARCHIES extended.
	// /usr and /etc are the implicit defaults systemd already searches, so we
	// prepend them to the operator-supplied list.
	hierarchyFiles := []interface{}{}
	if len(p.sysextHierarchies) > 0 {
		all := append([]string{"/usr"}, p.sysextHierarchies...)
		hierarchyFiles = append(hierarchyFiles, map[string]interface{}{
			"path":        "/etc/systemd/system/systemd-sysext.service.d/99-aurora-hierarchies.conf",
			"permissions": 0o644,
			"content":     "[Service]\nEnvironment=SYSTEMD_SYSEXT_HIERARCHIES=" + strings.Join(all, ":") + "\n",
		})
	}
	if len(p.confextHierarchies) > 0 {
		all := append([]string{"/etc"}, p.confextHierarchies...)
		hierarchyFiles = append(hierarchyFiles, map[string]interface{}{
			"path":        "/etc/systemd/system/systemd-confext.service.d/99-aurora-hierarchies.conf",
			"permissions": 0o644,
			"content":     "[Service]\nEnvironment=SYSTEMD_CONFEXT_HIERARCHIES=" + strings.Join(all, ":") + "\n",
		})
	}
	if len(hierarchyFiles) > 0 {
		stagesMap, _ := doc["stages"].(map[string]interface{})
		if stagesMap == nil {
			stagesMap = map[string]interface{}{}
			doc["stages"] = stagesMap
		}
		initramfs, _ := stagesMap["initramfs"].([]interface{})
		initramfs = append(initramfs, map[string]interface{}{"files": hierarchyFiles})
		stagesMap["initramfs"] = initramfs
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

// ListBundleExtensions handles GET /api/v1/artifacts/:id/bundle-extensions.
//
//	@Summary	List bundled extensions for an artifact
//	@Tags		Artifacts
//	@Produce	json
//	@Security	AdminBearer
//	@Param		id	path	string	true	"Artifact ID"
//	@Success	200	{array}	store.ArtifactExtensionBundle
//	@Router		/api/v1/artifacts/{id}/bundle-extensions [get]
func (h *ArtifactHandler) ListBundleExtensions(c echo.Context) error {
	if h.bundles == nil {
		return c.JSON(http.StatusOK, []store.ArtifactExtensionBundle{})
	}
	entries, err := h.bundles.ListForArtifact(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "list failed"})
	}
	if entries == nil {
		entries = []store.ArtifactExtensionBundle{}
	}
	return c.JSON(http.StatusOK, entries)
}

// setBundleEntry is the request shape for PUT /bundle-extensions.
type setBundleEntry struct {
	ExtensionName string `json:"extensionName"`
	ExtensionType string `json:"extensionType"`
	PinnedVersion string `json:"pinnedVersion,omitempty"`
	Order         int    `json:"order,omitempty"`
}

// SetBundleExtensions handles PUT /api/v1/artifacts/:id/bundle-extensions.
//
//	@Summary	Replace bundled extensions for an artifact
//	@Tags		Artifacts
//	@Accept		json
//	@Produce	json
//	@Security	AdminBearer
//	@Param		id		path	string			true	"Artifact ID"
//	@Param		body	body	[]setBundleEntry	true	"Replacement set"
//	@Success	200		{array}	store.ArtifactExtensionBundle
//	@Failure	400		{object}	APIError
//	@Failure	404		{object}	APIError
//	@Router		/api/v1/artifacts/{id}/bundle-extensions [put]
func (h *ArtifactHandler) SetBundleExtensions(c echo.Context) error {
	if h.bundles == nil || h.extensions == nil || h.store == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "bundles not configured"})
	}
	id := c.Param("id")
	ctx := c.Request().Context()

	artifact, err := h.store.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}

	var entries []setBundleEntry
	if err := c.Bind(&entries); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid body"})
	}

	out := make([]store.ArtifactExtensionBundle, 0, len(entries))
	for i, e := range entries {
		if e.ExtensionName == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("[%d]: extensionName required", i)})
		}
		if e.ExtensionType != "sysext" && e.ExtensionType != "confext" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("[%d]: extensionType must be sysext or confext", i)})
		}

		var ext *store.ExtensionRecord
		var rErr error
		if e.PinnedVersion != "" {
			ext, rErr = h.extensions.FindByNameAndVersion(ctx, e.ExtensionType, e.ExtensionName, e.PinnedVersion)
		} else {
			ext, rErr = h.extensions.FindLatestReadyByName(ctx, e.ExtensionType, e.ExtensionName)
		}
		if rErr != nil || ext == nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("[%d]: no Ready %s extension matches name=%q version=%q",
					i, e.ExtensionType, e.ExtensionName, e.PinnedVersion),
			})
		}
		if ext.Arch != artifact.Arch {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("[%d]: extension arch %q does not match artifact arch %q",
					i, ext.Arch, artifact.Arch),
			})
		}
		out = append(out, store.ArtifactExtensionBundle{
			ArtifactID:    id,
			ExtensionName: e.ExtensionName,
			ExtensionType: e.ExtensionType,
			PinnedVersion: e.PinnedVersion,
			Order:         e.Order,
		})
	}

	if err := h.bundles.ReplaceForArtifact(ctx, id, out); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "replace failed"})
	}
	return c.JSON(http.StatusOK, out)
}

// ResolvedBundleEntry is what the UI feeds back into the upgrade command's
// `extensions` arg. The agent will parse this same shape on the node.
type ResolvedBundleEntry struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Version string `json:"version"`
	Source  string `json:"source"`
}

// ResolveBundle handles POST /api/v1/artifacts/:id/bundle-resolve.
//
//	@Summary		Resolve bundled extensions for upgrade dispatch
//	@Description	Returns the bundle entries with concrete download URLs and resolved versions, ready to be passed as the `extensions` arg of an `upgrade` phonehome command.
//	@Tags			Artifacts
//	@Produce		json
//	@Security		AdminBearer
//	@Param			id	path	string	true	"Artifact ID"
//	@Success		200	{array}	ResolvedBundleEntry
//	@Failure		400	{object}	APIError
//	@Failure		404	{object}	APIError
//	@Router			/api/v1/artifacts/{id}/bundle-resolve [post]
func (h *ArtifactHandler) ResolveBundle(c echo.Context) error {
	if h.bundles == nil || h.extensions == nil || h.store == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "bundles not configured"})
	}
	id := c.Param("id")
	ctx := c.Request().Context()

	if _, err := h.store.GetByID(ctx, id); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "not found"})
	}

	entries, err := h.bundles.ListForArtifact(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "list failed"})
	}

	out := make([]ResolvedBundleEntry, 0, len(entries))
	for i, e := range entries {
		var ext *store.ExtensionRecord
		var rErr error
		if e.PinnedVersion != "" {
			ext, rErr = h.extensions.FindByNameAndVersion(ctx, e.ExtensionType, e.ExtensionName, e.PinnedVersion)
		} else {
			ext, rErr = h.extensions.FindLatestReadyByName(ctx, e.ExtensionType, e.ExtensionName)
		}
		if rErr != nil || ext == nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("bundle[%d]: no Ready %s extension matches name=%q version=%q",
					i, e.ExtensionType, e.ExtensionName, e.PinnedVersion),
			})
		}
		source := fmt.Sprintf("%s/api/v1/extensions/%s/download/%s",
			strings.TrimRight(h.aurorabootURL, "/"), ext.ID, ext.RawFilename)
		out = append(out, ResolvedBundleEntry{
			Name:    ext.Name,
			Type:    ext.Type,
			Version: ext.Version,
			Source:  source,
		})
	}
	return c.JSON(http.StatusOK, out)
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

func boolPtr(v bool) *bool { return &v }

// ReconcileOrphanedArtifacts fails every ArtifactRecord still marked Pending or
// Building. A process restart orphans the goroutine driving an in-flight build,
// so on startup those rows can never reach a terminal state on their own; flip
// them to Error with an explanatory message. Safe to call once during bootstrap.
// Per-row Update failures are logged and skipped so a single bad row does not
// block the rest of the sweep; only a failure listing artifacts is fatal.
func ReconcileOrphanedArtifacts(ctx context.Context, artifacts store.ArtifactStore) error {
	recs, err := artifacts.List(ctx)
	if err != nil {
		return fmt.Errorf("listing artifacts: %w", err)
	}
	for _, rec := range recs {
		if rec.Phase != store.ArtifactPending && rec.Phase != store.ArtifactBuilding {
			continue
		}
		prevPhase := rec.Phase
		rec.Phase = store.ArtifactError
		rec.Message = "interrupted by server restart"
		rec.UpdatedAt = time.Now()
		if err := artifacts.Update(ctx, rec); err != nil {
			fmt.Fprintf(os.Stderr, "reconcile: failed to mark artifact %s (was %s) as Error: %v\n", rec.ID, prevPhase, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "reconcile: marked orphaned artifact %s (was %s) as Error\n", rec.ID, prevPhase)
	}
	return nil
}
