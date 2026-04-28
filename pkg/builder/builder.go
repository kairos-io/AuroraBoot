package builder

import "context"

// ImageSource describes the base image and its properties.
type ImageSource struct {
	BaseImage         string
	KairosVersion     string
	Model             string
	Arch              string // "amd64" or "arm64"
	Variant           string // "core" or "standard"
	KubernetesDistro  string
	KubernetesVersion string
}

// OutputOptions selects which artifact formats to produce.
type OutputOptions struct {
	ISO         bool
	CloudImage  bool
	Netboot     bool
	RawDisk     bool
	Tar         bool
	GCE         bool
	VHD         bool
	UKI         bool
	FIPS        bool
	TrustedBoot bool
}

// SigningOptions holds SecureBoot / UKI signing paths.
type SigningOptions struct {
	UKISecureBootKey    string
	UKISecureBootCert   string
	UKITPMPCRKey        string
	UKIPublicKeysDir    string
	UKISecureBootEnroll string
}

// ProvisioningOptions controls post-build provisioning behaviour.
type ProvisioningOptions struct {
	AutoInstall        bool
	RegisterAuroraBoot bool
	TargetGroupID      string
	// AllowedCommands is the explicit phonehome.allowed_commands list baked
	// into the cloud-config. The AuroraBoot backend substitutes the safe
	// default set when the caller leaves this nil, so the emitted YAML always
	// carries the key — nodes never inherit an implicit agent-side default.
	AllowedCommands []string
}

// BuildOptions describes what to build.
type BuildOptions struct {
	ID   string // unique build ID
	Name string // optional friendly name

	// Grouped options (preferred for new code).
	Source       ImageSource
	Outputs      OutputOptions
	Signing      SigningOptions
	Provisioning ProvisioningOptions

	// Legacy flat fields — kept for backward compatibility with existing callers.
	// New code should use the grouped sub-structs above instead.
	BaseImage         string // e.g., "quay.io/kairos/ubuntu:24.04-core-amd64-generic-v3.6.0"
	KairosVersion     string
	Model             string
	KubernetesDistro  string
	KubernetesVersion string
	FIPS              bool
	TrustedBoot       bool
	ISO               bool
	CloudImage        bool
	Netboot           bool

	CloudConfig     string // YAML cloud-config to bake in
	OutputDir       string // where to write artifacts

	// Customization options:
	OverlayRootfs   string // path to overlay dir (files copied on top of rootfs)
	Dockerfile      string // optional Dockerfile content (builds image via docker before ISO)
	BuildContextDir string // directory with files available to COPY in Dockerfile
	ExtendCmdline   string
	KairosInitImage string
}

// BuildStatus tracks the state of a build.
type BuildStatus struct {
	ID        string   `json:"id"`
	Phase     string   `json:"phase"` // Pending, Building, Ready, Error
	Message   string   `json:"message"`
	Artifacts []string `json:"artifacts"` // paths to built files
}

// Build phases.
const (
	BuildPending  = "Pending"
	BuildBuilding = "Building"
	BuildReady    = "Ready"
	BuildError    = "Error"
)

// ArtifactBuilder builds Kairos artifacts.
type ArtifactBuilder interface {
	Build(ctx context.Context, opts BuildOptions) (*BuildStatus, error)
	Status(ctx context.Context, id string) (*BuildStatus, error)
	List(ctx context.Context) ([]*BuildStatus, error)
	Cancel(ctx context.Context, id string) error
}
