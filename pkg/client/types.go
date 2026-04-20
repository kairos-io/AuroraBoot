package client

import "time"

// These types describe the JSON wire format of the AuroraBoot REST API.
// They intentionally live in pkg/client rather than being imported
// from internal/store so that third-party consumers can pull this
// package in without dragging the private GORM layer with them.
//
// If you change a wire shape server-side, update the matching DTO
// here and run `make openapi` to refresh the generated spec.

// NodePhase is the lifecycle state of a managed node.
type NodePhase string

const (
	NodePhasePending    NodePhase = "Pending"
	NodePhaseRegistered NodePhase = "Registered"
	NodePhaseOnline     NodePhase = "Online"
	NodePhaseOffline    NodePhase = "Offline"
)

// Node describes a Kairos node known to AuroraBoot.
type Node struct {
	ID            string            `json:"id"`
	MachineID     string            `json:"machineID"`
	Hostname      string            `json:"hostname"`
	GroupID       string            `json:"groupID,omitempty"`
	Group         *Group            `json:"group,omitempty"`
	Phase         NodePhase         `json:"phase"`
	LastHeartbeat *time.Time        `json:"lastHeartbeat,omitempty"`
	AgentVersion  string            `json:"agentVersion"`
	OSRelease     map[string]string `json:"osRelease,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// Group is a logical bucket of nodes (environment, cluster, role...).
type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	NodeCount   int       `json:"node_count,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// NodeRegisterRequest is the body of POST /api/v1/nodes/register.
type NodeRegisterRequest struct {
	RegistrationToken string `json:"registrationToken"`
	MachineID         string `json:"machineID"`
	Hostname          string `json:"hostname,omitempty"`
	AgentVersion      string `json:"agentVersion,omitempty"`
}

// NodeRegisterResponse is returned by POST /api/v1/nodes/register.
type NodeRegisterResponse struct {
	ID     string `json:"id"`
	APIKey string `json:"apiKey"`
}

// NodeHeartbeatRequest is the body of POST /api/v1/nodes/:nodeID/heartbeat.
type NodeHeartbeatRequest struct {
	AgentVersion string            `json:"agentVersion,omitempty"`
	OSRelease    map[string]string `json:"osRelease,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// NodeListOptions filters the GET /api/v1/nodes response.
type NodeListOptions struct {
	// GroupID filters by exact group ID.
	GroupID string
	// Label filters by a single "key:value" pair. The server supports
	// exactly one label filter per request.
	Label string
}

// CommandPhase is the lifecycle state of a queued remote command.
type CommandPhase string

const (
	CommandPhasePending   CommandPhase = "Pending"
	CommandPhaseDelivered CommandPhase = "Delivered"
	CommandPhaseRunning   CommandPhase = "Running"
	CommandPhaseCompleted CommandPhase = "Completed"
	CommandPhaseFailed    CommandPhase = "Failed"
	CommandPhaseExpired   CommandPhase = "Expired"
)

// NodeCommand describes a queued remote operation.
type NodeCommand struct {
	ID            string            `json:"id"`
	ManagedNodeID string            `json:"managedNodeID"`
	Command       string            `json:"command"`
	Args          map[string]string `json:"args,omitempty"`
	Phase         CommandPhase      `json:"phase"`
	Result        string            `json:"result,omitempty"`
	ExpiresAt     *time.Time        `json:"expiresAt,omitempty"`
	DeliveredAt   *time.Time        `json:"deliveredAt,omitempty"`
	CompletedAt   *time.Time        `json:"completedAt,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
}

// CreateCommandRequest is the body for single-node and group-wide
// command creation.
type CreateCommandRequest struct {
	Command string            `json:"command"`
	Args    map[string]string `json:"args,omitempty"`
}

// CommandSelector targets nodes for a bulk command operation.
// Fields are AND'ed; NodeIDs if set takes precedence over Group/Labels.
type CommandSelector struct {
	GroupID string            `json:"groupID,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
	NodeIDs []string          `json:"nodeIDs,omitempty"`
}

// BulkCommandRequest is the body of POST /api/v1/nodes/commands.
type BulkCommandRequest struct {
	Selector CommandSelector   `json:"selector"`
	Command  string            `json:"command"`
	Args     map[string]string `json:"args,omitempty"`
}

// UpdateCommandStatusRequest is the body used by agents to report
// command progress.
type UpdateCommandStatusRequest struct {
	Phase  CommandPhase `json:"phase"`
	Result string       `json:"result,omitempty"`
}

// ArtifactPhase is the lifecycle state of a build.
type ArtifactPhase string

const (
	ArtifactPhasePending  ArtifactPhase = "Pending"
	ArtifactPhaseBuilding ArtifactPhase = "Building"
	ArtifactPhaseReady    ArtifactPhase = "Ready"
	ArtifactPhaseError    ArtifactPhase = "Error"
)

// Artifact is the stored representation of a build. Note that
// `KairosVersion` is the user-facing artifact version (baked into the
// image and used for upgrade tracking), not the Kairos framework
// version of the base image.
type Artifact struct {
	ID               string        `json:"id"`
	Name             string        `json:"name,omitempty"`
	Saved            bool          `json:"saved,omitempty"`
	Phase            ArtifactPhase `json:"phase"`
	Message          string        `json:"message,omitempty"`
	BaseImage        string        `json:"baseImage"`
	KairosVersion    string        `json:"kairosVersion"`
	Model            string        `json:"model"`
	Arch             string        `json:"arch"`
	Variant          string        `json:"variant"`
	KubernetesDistro string        `json:"kubernetesDistro,omitempty"`
	ISO              bool          `json:"iso"`
	CloudImage       bool          `json:"cloudImage"`
	Netboot          bool          `json:"netboot"`
	RawDisk          bool          `json:"rawDisk"`
	Tar              bool          `json:"tar"`
	GCE              bool          `json:"gce"`
	VHD              bool          `json:"vhd"`
	UKI              bool          `json:"uki"`
	FIPS             bool          `json:"fips"`
	TrustedBoot      bool          `json:"trustedBoot"`
	AutoInstall      bool          `json:"autoInstall"`
	RegisterAuroraBoot bool          `json:"registerAuroraBoot"`
	Dockerfile       string        `json:"dockerfile,omitempty"`
	CloudConfig      string        `json:"cloudConfig,omitempty"`
	TargetGroupID    string        `json:"targetGroupId,omitempty"`
	ContainerImage   string        `json:"containerImage,omitempty"`
	Artifacts        []string      `json:"artifacts,omitempty"`
	CreatedAt        time.Time     `json:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
}

// CreateArtifactRequest is the body of POST /api/v1/artifacts.
// This is a large struct; most fields are optional and reasonable
// defaults are applied server-side. Mirrors internal handler DTO.
type CreateArtifactRequest struct {
	Name              string                 `json:"name,omitempty"`
	BaseImage         string                 `json:"baseImage,omitempty"`
	KairosVersion     string                 `json:"kairosVersion,omitempty"`
	Model             string                 `json:"model,omitempty"`
	Arch              string                 `json:"arch,omitempty"`
	Variant           string                 `json:"variant,omitempty"`
	KubernetesDistro  string                 `json:"kubernetesDistro,omitempty"`
	KubernetesVersion string                 `json:"kubernetesVersion,omitempty"`
	Dockerfile        string                 `json:"dockerfile,omitempty"`
	OverlayRootfs     string                 `json:"overlayRootfs,omitempty"`
	KairosInitImage   string                 `json:"kairosInitImage,omitempty"`
	Outputs           ArtifactOutputs        `json:"outputs"`
	Signing           ArtifactSigning        `json:"signing"`
	Provisioning      ArtifactProvisioning   `json:"provisioning"`
	CloudConfig       string                 `json:"cloudConfig,omitempty"`
	Extra             map[string]interface{} `json:"-"` // reserved for future fields
}

// ArtifactOutputs toggles build output formats.
type ArtifactOutputs struct {
	ISO         bool `json:"iso,omitempty"`
	CloudImage  bool `json:"cloudImage,omitempty"`
	Netboot     bool `json:"netboot,omitempty"`
	RawDisk     bool `json:"rawDisk,omitempty"`
	Tar         bool `json:"tar,omitempty"`
	GCE         bool `json:"gce,omitempty"`
	VHD         bool `json:"vhd,omitempty"`
	UKI         bool `json:"uki,omitempty"`
	FIPS        bool `json:"fips,omitempty"`
	TrustedBoot bool `json:"trustedBoot,omitempty"`
}

// ArtifactSigning describes UKI SecureBoot signing options.
type ArtifactSigning struct {
	UKIKeySetID         string `json:"ukiKeySetId,omitempty"`
	UKISecureBootKey    string `json:"ukiSecureBootKey,omitempty"`
	UKISecureBootCert   string `json:"ukiSecureBootCert,omitempty"`
	UKITPMPCRKey        string `json:"ukiTpmPcrKey,omitempty"`
	UKIPublicKeysDir    string `json:"ukiPublicKeysDir,omitempty"`
	UKISecureBootEnroll string `json:"ukiSecureBootEnroll,omitempty"`
}

// ArtifactProvisioning describes cloud-config injection options.
type ArtifactProvisioning struct {
	AutoInstall      bool   `json:"autoInstall"`
	RegisterAuroraBoot bool   `json:"registerAuroraBoot"`
	TargetGroupID    string `json:"targetGroupId,omitempty"`
	UserMode         string `json:"userMode,omitempty"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	SSHKeys          string `json:"sshKeys,omitempty"`
}

// UpdateArtifactRequest is the body of PATCH /api/v1/artifacts/:id.
type UpdateArtifactRequest struct {
	Name  string `json:"name,omitempty"`
	Saved *bool  `json:"saved,omitempty"`
}

// SecureBootKeySet is a generated or imported UKI signing key set.
type SecureBootKeySet struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	KeysDir          string    `json:"keysDir"`
	TPMPCRKeyPath    string    `json:"tpmPcrKeyPath"`
	SecureBootEnroll string    `json:"secureBootEnroll"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// GenerateKeySetRequest is the body of POST /api/v1/secureboot-keys/generate.
type GenerateKeySetRequest struct {
	Name             string `json:"name"`
	SecureBootEnroll string `json:"secureBootEnroll,omitempty"`
}

// RegistrationTokenResponse wraps the current registration token.
type RegistrationTokenResponse struct {
	RegistrationToken string `json:"registrationToken"`
}
