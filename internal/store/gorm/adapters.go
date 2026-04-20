package gorm

import (
	"context"

	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// NodeStoreAdapter adapts Store to the store.NodeStore interface.
type NodeStoreAdapter struct{ S *Store }

func (a *NodeStoreAdapter) Register(ctx context.Context, node *store.ManagedNode) error {
	return a.S.Register(ctx, node)
}
func (a *NodeStoreAdapter) GetByID(ctx context.Context, id string) (*store.ManagedNode, error) {
	return a.S.NodeGetByID(ctx, id)
}
func (a *NodeStoreAdapter) GetByMachineID(ctx context.Context, machineID string) (*store.ManagedNode, error) {
	return a.S.GetByMachineID(ctx, machineID)
}
func (a *NodeStoreAdapter) GetByAPIKey(ctx context.Context, apiKey string) (*store.ManagedNode, error) {
	return a.S.GetByAPIKey(ctx, apiKey)
}
func (a *NodeStoreAdapter) List(ctx context.Context) ([]*store.ManagedNode, error) {
	return a.S.NodeList(ctx)
}
func (a *NodeStoreAdapter) ListByGroup(ctx context.Context, groupID string) ([]*store.ManagedNode, error) {
	return a.S.ListByGroup(ctx, groupID)
}
func (a *NodeStoreAdapter) ListByLabels(ctx context.Context, labels map[string]string) ([]*store.ManagedNode, error) {
	return a.S.ListByLabels(ctx, labels)
}
func (a *NodeStoreAdapter) ListBySelector(ctx context.Context, sel store.CommandSelector) ([]*store.ManagedNode, error) {
	return a.S.ListBySelector(ctx, sel)
}
func (a *NodeStoreAdapter) UpdateHeartbeat(ctx context.Context, id string, agentVersion string, osRelease map[string]string) error {
	return a.S.UpdateHeartbeat(ctx, id, agentVersion, osRelease)
}
func (a *NodeStoreAdapter) UpdatePhase(ctx context.Context, id string, phase string) error {
	return a.S.UpdatePhase(ctx, id, phase)
}
func (a *NodeStoreAdapter) SetGroup(ctx context.Context, nodeID string, groupID string) error {
	return a.S.SetGroup(ctx, nodeID, groupID)
}
func (a *NodeStoreAdapter) SetLabels(ctx context.Context, nodeID string, labels map[string]string) error {
	return a.S.SetLabels(ctx, nodeID, labels)
}
func (a *NodeStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.S.NodeDelete(ctx, id)
}

// CommandStoreAdapter adapts Store to the store.CommandStore interface.
type CommandStoreAdapter struct{ S *Store }

func (a *CommandStoreAdapter) Create(ctx context.Context, cmd *store.NodeCommand) error {
	return a.S.CommandCreate(ctx, cmd)
}
func (a *CommandStoreAdapter) GetByID(ctx context.Context, id string) (*store.NodeCommand, error) {
	return a.S.CommandGetByID(ctx, id)
}
func (a *CommandStoreAdapter) GetPending(ctx context.Context, nodeID string) ([]*store.NodeCommand, error) {
	return a.S.GetPending(ctx, nodeID)
}
func (a *CommandStoreAdapter) MarkDelivered(ctx context.Context, ids []string) error {
	return a.S.MarkDelivered(ctx, ids)
}
func (a *CommandStoreAdapter) UpdateStatus(ctx context.Context, id string, phase string, result string) error {
	return a.S.UpdateStatus(ctx, id, phase, result)
}
func (a *CommandStoreAdapter) ListByNode(ctx context.Context, nodeID string) ([]*store.NodeCommand, error) {
	return a.S.ListByNode(ctx, nodeID)
}
func (a *CommandStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.S.CommandDelete(ctx, id)
}
func (a *CommandStoreAdapter) DeleteTerminal(ctx context.Context, nodeID string) error {
	return a.S.CommandDeleteTerminal(ctx, nodeID)
}

// ArtifactStoreAdapter adapts Store to the store.ArtifactStore interface.
type ArtifactStoreAdapter struct{ S *Store }

func (a *ArtifactStoreAdapter) Create(ctx context.Context, rec *store.ArtifactRecord) error {
	return a.S.ArtifactCreate(ctx, rec)
}
func (a *ArtifactStoreAdapter) GetByID(ctx context.Context, id string) (*store.ArtifactRecord, error) {
	return a.S.ArtifactGetByID(ctx, id)
}
func (a *ArtifactStoreAdapter) List(ctx context.Context) ([]*store.ArtifactRecord, error) {
	return a.S.ArtifactList(ctx)
}
func (a *ArtifactStoreAdapter) Update(ctx context.Context, rec *store.ArtifactRecord) error {
	return a.S.ArtifactUpdate(ctx, rec)
}
func (a *ArtifactStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.S.ArtifactDelete(ctx, id)
}
func (a *ArtifactStoreAdapter) GetLogs(ctx context.Context, id string) (string, error) {
	return a.S.ArtifactGetLogs(ctx, id)
}
func (a *ArtifactStoreAdapter) AppendLog(ctx context.Context, id string, text string) error {
	return a.S.ArtifactAppendLog(ctx, id, text)
}
func (a *ArtifactStoreAdapter) DeleteByPhase(ctx context.Context, phase string) error {
	return a.S.ArtifactDeleteByPhase(ctx, phase)
}

// GroupStoreAdapter adapts Store to the store.GroupStore interface.
// The Store's GroupStore methods (Create, GetByID, GetByName, List, Update, Delete)
// already have the correct signatures, so this is a thin pass-through.
type GroupStoreAdapter struct{ S *Store }

func (a *GroupStoreAdapter) Create(ctx context.Context, group *store.NodeGroup) error {
	return a.S.Create(ctx, group)
}
func (a *GroupStoreAdapter) GetByID(ctx context.Context, id string) (*store.NodeGroup, error) {
	return a.S.GetByID(ctx, id)
}
func (a *GroupStoreAdapter) GetByName(ctx context.Context, name string) (*store.NodeGroup, error) {
	return a.S.GetByName(ctx, name)
}
func (a *GroupStoreAdapter) List(ctx context.Context) ([]*store.NodeGroup, error) {
	return a.S.List(ctx)
}
func (a *GroupStoreAdapter) Update(ctx context.Context, group *store.NodeGroup) error {
	return a.S.Update(ctx, group)
}
func (a *GroupStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.S.Delete(ctx, id)
}

// SecureBootKeySetStoreAdapter adapts Store to the store.SecureBootKeySetStore interface.
type SecureBootKeySetStoreAdapter struct{ S *Store }

func (a *SecureBootKeySetStoreAdapter) Create(ctx context.Context, ks *store.SecureBootKeySet) error {
	return a.S.SecureBootKeySetCreate(ctx, ks)
}
func (a *SecureBootKeySetStoreAdapter) GetByID(ctx context.Context, id string) (*store.SecureBootKeySet, error) {
	return a.S.SecureBootKeySetGetByID(ctx, id)
}
func (a *SecureBootKeySetStoreAdapter) GetByName(ctx context.Context, name string) (*store.SecureBootKeySet, error) {
	return a.S.SecureBootKeySetGetByName(ctx, name)
}
func (a *SecureBootKeySetStoreAdapter) List(ctx context.Context) ([]*store.SecureBootKeySet, error) {
	return a.S.SecureBootKeySetList(ctx)
}
func (a *SecureBootKeySetStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.S.SecureBootKeySetDelete(ctx, id)
}

// BMCTargetStoreAdapter adapts Store to the store.BMCTargetStore interface.
type BMCTargetStoreAdapter struct{ S *Store }

func (a *BMCTargetStoreAdapter) Create(ctx context.Context, target *store.BMCTarget) error {
	return a.S.BMCTargetCreate(ctx, target)
}
func (a *BMCTargetStoreAdapter) GetByID(ctx context.Context, id string) (*store.BMCTarget, error) {
	return a.S.BMCTargetGetByID(ctx, id)
}
func (a *BMCTargetStoreAdapter) List(ctx context.Context) ([]*store.BMCTarget, error) {
	return a.S.BMCTargetList(ctx)
}
func (a *BMCTargetStoreAdapter) Update(ctx context.Context, target *store.BMCTarget) error {
	return a.S.BMCTargetUpdate(ctx, target)
}
func (a *BMCTargetStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.S.BMCTargetDelete(ctx, id)
}

// DeploymentStoreAdapter adapts Store to the store.DeploymentStore interface.
type DeploymentStoreAdapter struct{ S *Store }

func (a *DeploymentStoreAdapter) Create(ctx context.Context, dep *store.Deployment) error {
	return a.S.DeploymentCreate(ctx, dep)
}
func (a *DeploymentStoreAdapter) GetByID(ctx context.Context, id string) (*store.Deployment, error) {
	return a.S.DeploymentGetByID(ctx, id)
}
func (a *DeploymentStoreAdapter) List(ctx context.Context) ([]*store.Deployment, error) {
	return a.S.DeploymentList(ctx)
}
func (a *DeploymentStoreAdapter) ListByArtifact(ctx context.Context, artifactID string) ([]*store.Deployment, error) {
	return a.S.DeploymentListByArtifact(ctx, artifactID)
}
func (a *DeploymentStoreAdapter) Update(ctx context.Context, dep *store.Deployment) error {
	return a.S.DeploymentUpdate(ctx, dep)
}
func (a *DeploymentStoreAdapter) Delete(ctx context.Context, id string) error {
	return a.S.DeploymentDelete(ctx, id)
}
