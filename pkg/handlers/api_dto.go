package handlers

// This file defines the exported request/response types that swag
// scans when generating the OpenAPI spec. The actual handlers continue
// to use whatever internal shapes they like — these DTOs exist purely
// so the generated docs can reference concrete, named JSON schemas
// instead of `any` or `map[string]string`.
//
// Keep these in sync with the handler implementations: if you change a
// request body shape, update the matching DTO here and re-run
// `make openapi` so the generated spec stays accurate.

import (
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// --- Error envelope ---

// APIError is the standard error body returned when a handler rejects a
// request.
type APIError struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

// --- Nodes ---

// APIRegisterRequest is the JSON body of POST /api/v1/nodes/register.
type APIRegisterRequest struct {
	RegistrationToken string `json:"registrationToken" example:"74a7e7452f9da2fd7739557f027bca0f"`
	MachineID         string `json:"machineID" example:"a1b2c3d4e5f6"`
	Hostname          string `json:"hostname" example:"kairos-node-01"`
	AgentVersion      string `json:"agentVersion" example:"v2.27.0"`
}

// APIRegisterResponse is the JSON body returned by POST /api/v1/nodes/register.
type APIRegisterResponse struct {
	ID     string `json:"id" example:"c8a4fb46-1836-4c70-97c8-29490ad110bc"`
	APIKey string `json:"apiKey" example:"3db2c1e4f5a6b7c8d9e0f1a2b3c4d5e6"`
}

// APIHeartbeatRequest is the JSON body of POST /api/v1/nodes/:nodeID/heartbeat.
type APIHeartbeatRequest struct {
	AgentVersion string            `json:"agentVersion" example:"v2.27.0"`
	OSRelease    map[string]string `json:"osRelease"`
	Labels       map[string]string `json:"labels"`
}

// APISetLabelsRequest is the JSON body of PUT /api/v1/nodes/:nodeID/labels.
type APISetLabelsRequest struct {
	Labels map[string]string `json:"labels"`
}

// APISetGroupRequest is the JSON body of PUT /api/v1/nodes/:nodeID/group.
type APISetGroupRequest struct {
	GroupID string `json:"groupID" example:"b1db4f35-6195-410a-94f2-09e66e0a6a83"`
}

// --- Groups ---

// APICreateGroupRequest is the JSON body of POST /api/v1/groups.
type APICreateGroupRequest struct {
	Name        string `json:"name" example:"production"`
	Description string `json:"description" example:"Production fleet nodes"`
}

// APIUpdateGroupRequest is the JSON body of PUT /api/v1/groups/:id.
type APIUpdateGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// --- Commands ---

// APICreateCommandRequest is the JSON body of
// POST /api/v1/nodes/:nodeID/commands and POST /api/v1/groups/:id/commands.
type APICreateCommandRequest struct {
	Command string            `json:"command" example:"upgrade" enums:"upgrade,upgrade-recovery,reset,apply-cloud-config,reboot,exec"`
	Args    map[string]string `json:"args"`
}

// APIBulkCommandRequest is the JSON body of POST /api/v1/nodes/commands.
type APIBulkCommandRequest struct {
	Selector store.CommandSelector `json:"selector"`
	Command  string                `json:"command" example:"upgrade"`
	Args     map[string]string     `json:"args"`
}

// APIUpdateCommandStatusRequest is the JSON body of
// PUT /api/v1/nodes/:nodeID/commands/:commandID/status.
type APIUpdateCommandStatusRequest struct {
	Phase  string `json:"phase" enums:"Pending,Delivered,Running,Completed,Failed,Expired"`
	Result string `json:"result"`
}

// --- Artifacts ---

// APICreateArtifactRequest is the JSON body of POST /api/v1/artifacts.
// Mirrors the shape the frontend sends; see internal/handlers/artifacts.go
// for the authoritative parser.
type APICreateArtifactRequest struct {
	Name              string                  `json:"name"`
	BaseImage         string                  `json:"baseImage"`
	KairosVersion     string                  `json:"kairosVersion" example:"v1.0"`
	Model             string                  `json:"model" example:"generic"`
	Arch              string                  `json:"arch" example:"amd64" enums:"amd64,arm64"`
	Variant           string                  `json:"variant" example:"core" enums:"core,standard"`
	KubernetesDistro  string                  `json:"kubernetesDistro" enums:"k3s,k0s"`
	KubernetesVersion string                  `json:"kubernetesVersion"`
	Dockerfile        string                  `json:"dockerfile"`
	OverlayRootfs     string                  `json:"overlayRootfs"`
	KairosInitImage   string                  `json:"kairosInitImage"`
	Outputs           APIArtifactOutputs      `json:"outputs"`
	Signing           APIArtifactSigning      `json:"signing"`
	Provisioning      APIArtifactProvisioning `json:"provisioning"`
	CloudConfig       string                  `json:"cloudConfig"`
}

// APIArtifactOutputs toggles the build's output formats.
type APIArtifactOutputs struct {
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

// APIArtifactSigning holds SecureBoot signing options for UKI builds.
type APIArtifactSigning struct {
	UKIKeySetID         string `json:"ukiKeySetId"`
	UKISecureBootKey    string `json:"ukiSecureBootKey"`
	UKISecureBootCert   string `json:"ukiSecureBootCert"`
	UKITPMPCRKey        string `json:"ukiTpmPcrKey"`
	UKIPublicKeysDir    string `json:"ukiPublicKeysDir"`
	UKISecureBootEnroll string `json:"ukiSecureBootEnroll" enums:"off,manual,if-safe,force"`
}

// APIArtifactProvisioning holds cloud-config injection options.
type APIArtifactProvisioning struct {
	AutoInstall        bool     `json:"autoInstall"`
	RegisterAuroraBoot bool     `json:"registerAuroraBoot"`
	TargetGroupID      string   `json:"targetGroupId"`
	UserMode           string   `json:"userMode" enums:"default,custom,none"`
	Username           string   `json:"username"`
	Password           string   `json:"password"`
	SSHKeys            string   `json:"sshKeys"`
	// AllowedCommands is the explicit list emitted under phonehome.allowed_commands.
	// Nil means "use AuroraBoot's safe default set". An empty slice means deny-all
	// (observe-only node) — the UI warns when the operator picks this.
	AllowedCommands []string `json:"allowedCommands"`
}

// APIUpdateArtifactRequest is the JSON body of PATCH /api/v1/artifacts/:id.
type APIUpdateArtifactRequest struct {
	Name  string `json:"name"`
	Saved *bool  `json:"saved"`
}

// --- SecureBoot keys ---

// APIGenerateKeySetRequest is the JSON body of
// POST /api/v1/secureboot-keys/generate.
type APIGenerateKeySetRequest struct {
	Name             string `json:"name" example:"production"`
	SecureBootEnroll string `json:"secureBootEnroll" enums:"off,manual,if-safe,force"`
}

// --- Settings ---

// APIRegistrationTokenResponse wraps the registration token for the
// settings endpoints.
type APIRegistrationTokenResponse struct {
	RegistrationToken string `json:"registrationToken"`
}

// Verify time is imported — otherwise unused-import error.
var _ = time.Time{}
