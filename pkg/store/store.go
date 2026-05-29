package store

import (
	"context"
	"time"
)

// NodeGroup represents a logical group/environment for nodes (e.g., "production", "staging").
type NodeGroup struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"uniqueIndex"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ManagedNode represents a Kairos node managed by auroraboot.
type ManagedNode struct {
	ID            string            `json:"id" gorm:"primaryKey"`
	MachineID     string            `json:"machineID" gorm:"uniqueIndex"`
	Hostname      string            `json:"hostname"`
	GroupID       string            `json:"groupID" gorm:"index"`
	Group         *NodeGroup        `json:"group,omitempty" gorm:"foreignKey:GroupID;constraint:OnDelete:SET NULL"`
	Phase         string            `json:"phase"`
	LastHeartbeat *time.Time        `json:"lastHeartbeat"`
	AgentVersion  string            `json:"agentVersion"`
	OSRelease     map[string]string `json:"osRelease" gorm:"serializer:json"`
	Labels        map[string]string `json:"labels" gorm:"serializer:json"`
	APIKey        string            `json:"-" gorm:"index"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// Node phases.
const (
	PhasePending    = "Pending"
	PhaseRegistered = "Registered"
	PhaseOnline     = "Online"
	PhaseOffline    = "Offline"
)

// NodeCommand represents a command queued for a node.
type NodeCommand struct {
	ID            string            `json:"id" gorm:"primaryKey"`
	ManagedNodeID string            `json:"managedNodeID" gorm:"index"`
	Command       string            `json:"command"`
	Args          map[string]string `json:"args" gorm:"serializer:json"`
	Phase         string            `json:"phase"`
	Result        string            `json:"result"`
	ExpiresAt     *time.Time        `json:"expiresAt"`
	DeliveredAt   *time.Time        `json:"deliveredAt"`
	CompletedAt   *time.Time        `json:"completedAt"`
	CreatedAt     time.Time         `json:"createdAt"`
}

// Command phases.
const (
	CommandPending   = "Pending"
	CommandDelivered = "Delivered"
	CommandRunning   = "Running"
	CommandCompleted = "Completed"
	CommandFailed    = "Failed"
	CommandExpired   = "Expired"
)

// Command types.
const (
	CmdUpgrade          = "upgrade"
	CmdReset            = "reset"
	CmdExec             = "exec"
	CmdApplyCloudConfig = "apply-cloud-config"
	CmdUpgradeRecovery  = "upgrade-recovery"
	CmdReboot           = "reboot"
	CmdExtension        = "extension"
)

// CommandSelector targets nodes for bulk command operations.
type CommandSelector struct {
	GroupID string            `json:"groupID,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
	NodeIDs []string          `json:"nodeIDs,omitempty"`
}

// GroupStore manages node groups.
type GroupStore interface {
	Create(ctx context.Context, group *NodeGroup) error
	GetByID(ctx context.Context, id string) (*NodeGroup, error)
	GetByName(ctx context.Context, name string) (*NodeGroup, error)
	List(ctx context.Context) ([]*NodeGroup, error)
	Update(ctx context.Context, group *NodeGroup) error
	Delete(ctx context.Context, id string) error
}

// NodeStore manages node registration and state.
type NodeStore interface {
	Register(ctx context.Context, node *ManagedNode) error
	GetByID(ctx context.Context, id string) (*ManagedNode, error)
	GetByMachineID(ctx context.Context, machineID string) (*ManagedNode, error)
	GetByAPIKey(ctx context.Context, apiKey string) (*ManagedNode, error)
	List(ctx context.Context) ([]*ManagedNode, error)
	ListByGroup(ctx context.Context, groupID string) ([]*ManagedNode, error)
	ListByLabels(ctx context.Context, labels map[string]string) ([]*ManagedNode, error)
	ListBySelector(ctx context.Context, sel CommandSelector) ([]*ManagedNode, error)
	UpdateHeartbeat(ctx context.Context, id string, agentVersion string, osRelease map[string]string) error
	UpdatePhase(ctx context.Context, id string, phase string) error
	SetGroup(ctx context.Context, nodeID string, groupID string) error
	SetLabels(ctx context.Context, nodeID string, labels map[string]string) error
	Delete(ctx context.Context, id string) error
}

// CommandStore manages the command queue.
type CommandStore interface {
	Create(ctx context.Context, cmd *NodeCommand) error
	GetByID(ctx context.Context, id string) (*NodeCommand, error)
	GetPending(ctx context.Context, nodeID string) ([]*NodeCommand, error)
	MarkDelivered(ctx context.Context, ids []string) error
	UpdateStatus(ctx context.Context, id string, phase string, result string) error
	ListByNode(ctx context.Context, nodeID string) ([]*NodeCommand, error)
	Delete(ctx context.Context, id string) error
	DeleteTerminal(ctx context.Context, nodeID string) error
}

// ArtifactRecord stores a build artifact and its metadata.
type ArtifactRecord struct {
	ID                   string               `json:"id" gorm:"primaryKey"`
	Name                 string               `json:"name,omitempty"`
	Saved                bool                 `json:"saved,omitempty"`
	Phase                string               `json:"phase"`
	Message              string               `json:"message"`
	BaseImage            string               `json:"baseImage"`
	KairosVersion        string               `json:"kairosVersion"`
	Model                string               `json:"model"`
	ISO                  bool                 `json:"iso"`
	CloudImage           bool                 `json:"cloudImage"`
	Netboot              bool                 `json:"netboot"`
	FIPS                 bool                 `json:"fips"`
	TrustedBoot          bool                 `json:"trustedBoot"`
	Arch                 string               `json:"arch,omitempty"`
	Variant              string               `json:"variant,omitempty"`
	RawDisk              bool                 `json:"rawDisk"`
	Tar                  bool                 `json:"tar"`
	GCE                  bool                 `json:"gce"`
	VHD                  bool                 `json:"vhd"`
	UKI                  bool                 `json:"uki"`
	KairosInitImage      string               `json:"kairosInitImage,omitempty"`
	AutoInstall          bool                 `json:"autoInstall"`
	RegisterAuroraBoot   bool                 `json:"registerAuroraBoot"`
	Dockerfile           string               `json:"dockerfile,omitempty"`
	CloudConfig          string               `json:"cloudConfig,omitempty" gorm:"type:text"`
	KubernetesDistro     string               `json:"kubernetesDistro,omitempty"`
	KubernetesVersion    string               `json:"kubernetesVersion,omitempty"`
	TargetGroupID        string               `json:"targetGroupId,omitempty"`
	ContainerImage       string               `json:"containerImage,omitempty"`
	OverlayRootfs        string               `json:"overlayRootfs,omitempty"`
	ArtifactFiles        []string             `json:"artifacts" gorm:"serializer:json"`
	ExtensionHierarchies ExtensionHierarchies `gorm:"serializer:json" json:"extensionHierarchies"`
	Logs                 string               `json:"-" gorm:"type:text"`
	CreatedAt            time.Time            `json:"createdAt"`
	UpdatedAt            time.Time            `json:"updatedAt"`
}

// ExtensionHierarchies records the SYSTEMD_{SYSEXT,CONFEXT}_HIERARCHIES paths
// declared at artifact build time so the Extensions UI can cross-check what
// scopes an OS image supports. /usr (sysext) and /etc (confext) are implicit
// and never stored in either slice.
type ExtensionHierarchies struct {
	Sysext  []string `json:"sysext"`
	Confext []string `json:"confext"`
}

// Artifact phases.
const (
	ArtifactPending  = "Pending"
	ArtifactBuilding = "Building"
	ArtifactReady    = "Ready"
	ArtifactError    = "Error"
)

// ExtensionRecord is one sysext or confext build managed by AuroraBoot.
// .raw output lives at <artifactsDir>/extensions/<ID>/<Name>.<Type>.raw.
type ExtensionRecord struct {
	ID      string `gorm:"primaryKey" json:"id"`
	Name    string `gorm:"index"      json:"name"`
	Type    string `json:"type"`  // "sysext" | "confext"
	Phase   string `json:"phase"` // Pending | Building | Ready | Error
	Message string `json:"message"`

	Arch    string `json:"arch"`
	Version string `json:"version"`

	SourceMode       string `json:"sourceMode"` // artifact | image | dockerfile
	SourceArtifactID string `json:"sourceArtifactId"`
	SourceImage      string `json:"sourceImage"`
	Dockerfile       string `gorm:"type:text" json:"dockerfile,omitempty"`
	ExtraSteps       string `gorm:"type:text" json:"extraSteps,omitempty"`

	SigningKeySetID string   `json:"signingKeySetId"`
	Hierarchies     []string `gorm:"serializer:json" json:"hierarchies"`
	ServiceReload   bool     `json:"serviceReload"`

	ContainerImage string `json:"containerImage"`
	RawFilename    string `json:"rawFilename"`

	Logs string `gorm:"type:text" json:"-"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// NodeExtensionRow is the per-node tracking that drives the Install dialog's
// pre-action diff and the node detail page's "Installed extensions" section.
// The agent's REST status callback writes/deletes rows on each successful
// install / disable / remove.
type NodeExtensionRow struct {
	NodeID      string    `gorm:"primaryKey" json:"nodeId"`
	Name        string    `gorm:"primaryKey" json:"name"`
	Type        string    `gorm:"primaryKey" json:"type"`      // sysext | confext
	BootState   string    `gorm:"primaryKey" json:"bootState"` // active | passive | recovery | common
	ExtensionID string    `                  json:"extensionId,omitempty"`
	Version     string    `                  json:"version"`
	InstalledAt time.Time `                  json:"installedAt"`
	UpdatedAt   time.Time `                  json:"updatedAt"`
}

// ArtifactExtensionBundle links an artifact to an extension that should ride
// with every upgrade to that artifact. Entries are by (ArtifactID,
// ExtensionName) — the actual extension UUID is resolved at dispatch time so
// the bundle survives rebuilds of the named extension.
type ArtifactExtensionBundle struct {
	ArtifactID    string `gorm:"primaryKey" json:"artifactId"`
	ExtensionName string `gorm:"primaryKey" json:"extensionName"`
	ExtensionType string `                  json:"extensionType"` // sysext | confext
	PinnedVersion string `                  json:"pinnedVersion,omitempty"`
	Order         int    `                  json:"order"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ArtifactStore manages build artifact records.
type ArtifactStore interface {
	Create(ctx context.Context, rec *ArtifactRecord) error
	GetByID(ctx context.Context, id string) (*ArtifactRecord, error)
	List(ctx context.Context) ([]*ArtifactRecord, error)
	Update(ctx context.Context, rec *ArtifactRecord) error
	Delete(ctx context.Context, id string) error
	DeleteByPhase(ctx context.Context, phase string) error
	GetLogs(ctx context.Context, id string) (string, error)
	AppendLog(ctx context.Context, id string, text string) error
}

// ExtensionStore manages sysext / confext extension records.
type ExtensionStore interface {
	Create(ctx context.Context, e *ExtensionRecord) error
	GetByID(ctx context.Context, id string) (*ExtensionRecord, error)
	List(ctx context.Context) ([]ExtensionRecord, error)
	Delete(ctx context.Context, id string) error
	FindLatestReadyByName(ctx context.Context, extType, name string) (*ExtensionRecord, error)
	FindByNameAndVersion(ctx context.Context, extType, name, version string) (*ExtensionRecord, error)
	AppendLog(ctx context.Context, id, chunk string) error
}

// ArtifactExtensionBundleStore manages the per-artifact list of bundled
// extensions that ride along with every upgrade to that artifact.
type ArtifactExtensionBundleStore interface {
	ListForArtifact(ctx context.Context, artifactID string) ([]ArtifactExtensionBundle, error)
	ReplaceForArtifact(ctx context.Context, artifactID string, entries []ArtifactExtensionBundle) error
	ArtifactsReferencingExtension(ctx context.Context, extensionName string) ([]string, error)
}

// NodeExtensionStore manages per-node installed extension tracking rows
// updated by the agent's status callback on install / disable / remove.
type NodeExtensionStore interface {
	Upsert(ctx context.Context, row *NodeExtensionRow) error
	ListForNode(ctx context.Context, nodeID string) ([]NodeExtensionRow, error)
	ListForExtensionByName(ctx context.Context, extType, name string) ([]NodeExtensionRow, error)
	DeleteByScope(ctx context.Context, nodeID, extType, name, bootState string) error
	DeleteByName(ctx context.Context, nodeID, extType, name string) error
}

// SecureBootKeySet tracks a named set of SecureBoot keys on the filesystem.
type SecureBootKeySet struct {
	ID               string    `json:"id" gorm:"primaryKey"`
	Name             string    `json:"name" gorm:"uniqueIndex"`
	KeysDir          string    `json:"keysDir"`
	TPMPCRKeyPath    string    `json:"tpmPcrKeyPath"`
	SecureBootEnroll string    `json:"secureBootEnroll"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// SecureBootKeySetStore manages SecureBoot key sets.
type SecureBootKeySetStore interface {
	Create(ctx context.Context, ks *SecureBootKeySet) error
	GetByID(ctx context.Context, id string) (*SecureBootKeySet, error)
	GetByName(ctx context.Context, name string) (*SecureBootKeySet, error)
	List(ctx context.Context) ([]*SecureBootKeySet, error)
	Delete(ctx context.Context, id string) error
}

// BMCTarget stores saved RedFish/IPMI credentials for a baseboard management controller.
type BMCTarget struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name"`
	Endpoint  string    `json:"endpoint"`
	Vendor    string    `json:"vendor"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	VerifySSL bool      `json:"verifySSL"`
	NodeID    string    `json:"nodeId,omitempty" gorm:"index"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// BMCTargetStore manages BMC target records.
type BMCTargetStore interface {
	Create(ctx context.Context, target *BMCTarget) error
	GetByID(ctx context.Context, id string) (*BMCTarget, error)
	List(ctx context.Context) ([]*BMCTarget, error)
	Update(ctx context.Context, target *BMCTarget) error
	Delete(ctx context.Context, id string) error
}

// Deployment tracks a deployment operation.
type Deployment struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	ArtifactID  string     `json:"artifactId" gorm:"index"`
	Method      string     `json:"method"`
	Status      string     `json:"status"`
	Message     string     `json:"message"`
	BMCTargetID string     `json:"bmcTargetId,omitempty"`
	Progress    int        `json:"progress"`
	StartedAt   time.Time  `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// Deployment statuses.
const (
	DeployActive    = "Active"
	DeployCompleted = "Completed"
	DeployFailed    = "Failed"
)

// DeploymentStore manages deployment records.
type DeploymentStore interface {
	Create(ctx context.Context, dep *Deployment) error
	GetByID(ctx context.Context, id string) (*Deployment, error)
	List(ctx context.Context) ([]*Deployment, error)
	ListByArtifact(ctx context.Context, artifactID string) ([]*Deployment, error)
	Update(ctx context.Context, dep *Deployment) error
	Delete(ctx context.Context, id string) error
}
