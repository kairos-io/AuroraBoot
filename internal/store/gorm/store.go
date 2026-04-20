package gorm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"strings"

	"github.com/google/uuid"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Store implements GroupStore, NodeStore, and CommandStore using GORM.
// Supports SQLite (default) and PostgreSQL.
type Store struct {
	db *gorm.DB
}

// New creates a new Store with the given DSN and auto-migrates all models.
// The DSN format determines the database driver:
//   - "postgres://..." or "host=..." → PostgreSQL
//   - anything else → SQLite (file path or ":memory:")
func New(dsn string) (*Store, error) {
	dialector := openDialector(dsn)
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite-specific: enable WAL mode for concurrent access
	if !isPostgres(dsn) {
		sqlDB, err := db.DB()
		if err != nil {
			return nil, fmt.Errorf("getting sql.DB: %w", err)
		}
		if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
			return nil, fmt.Errorf("enabling WAL mode: %w", err)
		}
	}

	if err := db.AutoMigrate(&store.NodeGroup{}, &store.ManagedNode{}, &store.NodeCommand{}, &store.ArtifactRecord{}, &store.SecureBootKeySet{}, &store.BMCTarget{}, &store.Deployment{}); err != nil {
		return nil, fmt.Errorf("auto-migrating: %w", err)
	}

	return &Store{db: db}, nil
}

func isPostgres(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") ||
		strings.HasPrefix(dsn, "postgresql://") ||
		strings.HasPrefix(dsn, "host=")
}

func openDialector(dsn string) gorm.Dialector {
	if isPostgres(dsn) {
		return postgres.Open(dsn)
	}
	return sqlite.Open(dsn)
}

// --- GroupStore ---

func (s *Store) Create(ctx context.Context, group *store.NodeGroup) error {
	group.ID = uuid.New().String()
	return s.db.WithContext(ctx).Create(group).Error
}

func (s *Store) GetByID(ctx context.Context, id string) (*store.NodeGroup, error) {
	var g store.NodeGroup
	if err := s.db.WithContext(ctx).First(&g, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) GetByName(ctx context.Context, name string) (*store.NodeGroup, error) {
	var g store.NodeGroup
	if err := s.db.WithContext(ctx).First(&g, "name = ?", name).Error; err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) List(ctx context.Context) ([]*store.NodeGroup, error) {
	var groups []*store.NodeGroup
	if err := s.db.WithContext(ctx).Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *Store) Update(ctx context.Context, group *store.NodeGroup) error {
	return s.db.WithContext(ctx).Save(group).Error
}

// Delete removes a group and detaches any member nodes. The GORM tag on
// ManagedNode.Group sets up OnDelete:SET NULL, but SQLite doesn't enforce
// foreign keys unless PRAGMA foreign_keys=ON is set per-connection, so we
// nullify node.group_id explicitly in a transaction to behave the same on
// SQLite and Postgres.
func (s *Store) Delete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&store.ManagedNode{}).
			Where("group_id = ?", id).
			Update("group_id", "").Error; err != nil {
			return err
		}
		return tx.Delete(&store.NodeGroup{}, "id = ?", id).Error
	})
}

// --- NodeStore ---

func generateAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) Register(ctx context.Context, node *store.ManagedNode) error {
	node.ID = uuid.New().String()
	apiKey, err := generateAPIKey()
	if err != nil {
		return fmt.Errorf("generating API key: %w", err)
	}
	node.APIKey = apiKey
	node.Phase = store.PhaseRegistered
	return s.db.WithContext(ctx).Create(node).Error
}

func (s *Store) NodeGetByID(ctx context.Context, id string) (*store.ManagedNode, error) {
	var n store.ManagedNode
	if err := s.db.WithContext(ctx).Preload("Group").First(&n, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Store) GetByMachineID(ctx context.Context, machineID string) (*store.ManagedNode, error) {
	var n store.ManagedNode
	if err := s.db.WithContext(ctx).Preload("Group").First(&n, "machine_id = ?", machineID).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Store) GetByAPIKey(ctx context.Context, apiKey string) (*store.ManagedNode, error) {
	var n store.ManagedNode
	if err := s.db.WithContext(ctx).Preload("Group").First(&n, "api_key = ?", apiKey).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Store) NodeList(ctx context.Context) ([]*store.ManagedNode, error) {
	var nodes []*store.ManagedNode
	if err := s.db.WithContext(ctx).Preload("Group").Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *Store) ListByGroup(ctx context.Context, groupID string) ([]*store.ManagedNode, error) {
	var nodes []*store.ManagedNode
	if err := s.db.WithContext(ctx).Preload("Group").Where("group_id = ?", groupID).Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *Store) ListByLabels(ctx context.Context, labels map[string]string) ([]*store.ManagedNode, error) {
	var all []*store.ManagedNode
	if err := s.db.WithContext(ctx).Preload("Group").Find(&all).Error; err != nil {
		return nil, err
	}

	var result []*store.ManagedNode
	for _, n := range all {
		if matchLabels(n.Labels, labels) {
			result = append(result, n)
		}
	}
	return result, nil
}

func matchLabels(nodeLabels, selector map[string]string) bool {
	for k, v := range selector {
		if nodeLabels[k] != v {
			return false
		}
	}
	return true
}

func (s *Store) ListBySelector(ctx context.Context, sel store.CommandSelector) ([]*store.ManagedNode, error) {
	// Start with all nodes, applying SQL-level filters where possible.
	q := s.db.WithContext(ctx).Preload("Group")

	if sel.GroupID != "" {
		q = q.Where("group_id = ?", sel.GroupID)
	}
	if len(sel.NodeIDs) > 0 {
		q = q.Where("id IN ?", sel.NodeIDs)
	}

	var nodes []*store.ManagedNode
	if err := q.Find(&nodes).Error; err != nil {
		return nil, err
	}

	// Filter by labels in Go if needed.
	if len(sel.Labels) > 0 {
		var filtered []*store.ManagedNode
		for _, n := range nodes {
			if matchLabels(n.Labels, sel.Labels) {
				filtered = append(filtered, n)
			}
		}
		return filtered, nil
	}

	return nodes, nil
}

func (s *Store) UpdateHeartbeat(ctx context.Context, id string, agentVersion string, osRelease map[string]string) error {
	var n store.ManagedNode
	if err := s.db.WithContext(ctx).First(&n, "id = ?", id).Error; err != nil {
		return err
	}
	now := time.Now()
	n.LastHeartbeat = &now
	n.AgentVersion = agentVersion
	n.OSRelease = osRelease
	n.Phase = store.PhaseOnline
	return s.db.WithContext(ctx).Save(&n).Error
}

func (s *Store) UpdatePhase(ctx context.Context, id string, phase string) error {
	return s.db.WithContext(ctx).Model(&store.ManagedNode{}).Where("id = ?", id).Update("phase", phase).Error
}

func (s *Store) SetGroup(ctx context.Context, nodeID string, groupID string) error {
	return s.db.WithContext(ctx).Model(&store.ManagedNode{}).Where("id = ?", nodeID).Update("group_id", groupID).Error
}

func (s *Store) SetLabels(ctx context.Context, nodeID string, labels map[string]string) error {
	var n store.ManagedNode
	if err := s.db.WithContext(ctx).First(&n, "id = ?", nodeID).Error; err != nil {
		return err
	}
	n.Labels = labels
	return s.db.WithContext(ctx).Save(&n).Error
}

func (s *Store) NodeDelete(ctx context.Context, id string) error {
	// Delete associated commands first.
	if err := s.db.WithContext(ctx).Where("managed_node_id = ?", id).Delete(&store.NodeCommand{}).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(&store.ManagedNode{}, "id = ?", id).Error
}

// --- CommandStore ---

func (s *Store) CommandCreate(ctx context.Context, cmd *store.NodeCommand) error {
	cmd.ID = uuid.New().String()
	cmd.Phase = store.CommandPending
	return s.db.WithContext(ctx).Create(cmd).Error
}

func (s *Store) CommandGetByID(ctx context.Context, id string) (*store.NodeCommand, error) {
	var cmd store.NodeCommand
	if err := s.db.WithContext(ctx).First(&cmd, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &cmd, nil
}

func (s *Store) GetPending(ctx context.Context, nodeID string) ([]*store.NodeCommand, error) {
	var cmds []*store.NodeCommand
	if err := s.db.WithContext(ctx).
		Where("managed_node_id = ? AND phase = ? AND (expires_at IS NULL OR expires_at > ?)",
			nodeID, store.CommandPending, time.Now()).
		Find(&cmds).Error; err != nil {
		return nil, err
	}
	return cmds, nil
}

func (s *Store) MarkDelivered(ctx context.Context, ids []string) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&store.NodeCommand{}).Where("id IN ?", ids).Updates(map[string]any{
		"phase":        store.CommandDelivered,
		"delivered_at": &now,
	}).Error
}

func (s *Store) UpdateStatus(ctx context.Context, id string, phase string, result string) error {
	updates := map[string]any{
		"phase":  phase,
		"result": result,
	}
	if phase == store.CommandCompleted || phase == store.CommandFailed {
		now := time.Now()
		updates["completed_at"] = &now
	}
	return s.db.WithContext(ctx).Model(&store.NodeCommand{}).Where("id = ?", id).Updates(updates).Error
}

func (s *Store) ListByNode(ctx context.Context, nodeID string) ([]*store.NodeCommand, error) {
	var cmds []*store.NodeCommand
	if err := s.db.WithContext(ctx).Where("managed_node_id = ?", nodeID).Find(&cmds).Error; err != nil {
		return nil, err
	}
	return cmds, nil
}

func (s *Store) CommandDelete(ctx context.Context, id string) error {
	result := s.db.WithContext(ctx).Delete(&store.NodeCommand{}, "id = ?", id)
	if result.RowsAffected == 0 {
		return fmt.Errorf("command not found")
	}
	return result.Error
}

func (s *Store) CommandDeleteTerminal(ctx context.Context, nodeID string) error {
	return s.db.WithContext(ctx).Where(
		"managed_node_id = ? AND (phase = ? OR phase = ?)",
		nodeID, store.CommandCompleted, store.CommandFailed,
	).Delete(&store.NodeCommand{}).Error
}

// --- ArtifactStore ---

func (s *Store) ArtifactCreate(ctx context.Context, rec *store.ArtifactRecord) error {
	if rec.Phase == "" {
		rec.Phase = store.ArtifactPending
	}
	return s.db.WithContext(ctx).Create(rec).Error
}

func (s *Store) ArtifactGetByID(ctx context.Context, id string) (*store.ArtifactRecord, error) {
	var rec store.ArtifactRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *Store) ArtifactList(ctx context.Context) ([]*store.ArtifactRecord, error) {
	var records []*store.ArtifactRecord
	if err := s.db.WithContext(ctx).Omit("Logs").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (s *Store) ArtifactUpdate(ctx context.Context, rec *store.ArtifactRecord) error {
	return s.db.WithContext(ctx).Save(rec).Error
}

func (s *Store) ArtifactDelete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&store.ArtifactRecord{}, "id = ?", id).Error
}

func (s *Store) ArtifactGetLogs(ctx context.Context, id string) (string, error) {
	var rec store.ArtifactRecord
	if err := s.db.WithContext(ctx).Select("logs").First(&rec, "id = ?", id).Error; err != nil {
		return "", err
	}
	return rec.Logs, nil
}

func (s *Store) ArtifactDeleteByPhase(ctx context.Context, phase string) error {
	return s.db.WithContext(ctx).Where("phase = ?", phase).Delete(&store.ArtifactRecord{}).Error
}

func (s *Store) ArtifactAppendLog(ctx context.Context, id string, text string) error {
	return s.db.WithContext(ctx).Model(&store.ArtifactRecord{}).Where("id = ?", id).
		Update("logs", gorm.Expr("COALESCE(logs, '') || ?", text)).Error
}

// --- SecureBootKeySetStore ---

func (s *Store) SecureBootKeySetCreate(ctx context.Context, ks *store.SecureBootKeySet) error {
	ks.ID = uuid.New().String()
	return s.db.WithContext(ctx).Create(ks).Error
}

func (s *Store) SecureBootKeySetGetByID(ctx context.Context, id string) (*store.SecureBootKeySet, error) {
	var ks store.SecureBootKeySet
	if err := s.db.WithContext(ctx).First(&ks, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &ks, nil
}

func (s *Store) SecureBootKeySetGetByName(ctx context.Context, name string) (*store.SecureBootKeySet, error) {
	var ks store.SecureBootKeySet
	if err := s.db.WithContext(ctx).First(&ks, "name = ?", name).Error; err != nil {
		return nil, err
	}
	return &ks, nil
}

func (s *Store) SecureBootKeySetList(ctx context.Context) ([]*store.SecureBootKeySet, error) {
	var sets []*store.SecureBootKeySet
	if err := s.db.WithContext(ctx).Find(&sets).Error; err != nil {
		return nil, err
	}
	return sets, nil
}

func (s *Store) SecureBootKeySetDelete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&store.SecureBootKeySet{}, "id = ?", id).Error
}

// --- BMCTargetStore ---

func (s *Store) BMCTargetCreate(ctx context.Context, target *store.BMCTarget) error {
	target.ID = uuid.New().String()
	return s.db.WithContext(ctx).Create(target).Error
}

func (s *Store) BMCTargetGetByID(ctx context.Context, id string) (*store.BMCTarget, error) {
	var t store.BMCTarget
	if err := s.db.WithContext(ctx).First(&t, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) BMCTargetList(ctx context.Context) ([]*store.BMCTarget, error) {
	var targets []*store.BMCTarget
	if err := s.db.WithContext(ctx).Find(&targets).Error; err != nil {
		return nil, err
	}
	return targets, nil
}

func (s *Store) BMCTargetUpdate(ctx context.Context, target *store.BMCTarget) error {
	return s.db.WithContext(ctx).Save(target).Error
}

func (s *Store) BMCTargetDelete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&store.BMCTarget{}, "id = ?", id).Error
}

// --- DeploymentStore ---

func (s *Store) DeploymentCreate(ctx context.Context, dep *store.Deployment) error {
	dep.ID = uuid.New().String()
	return s.db.WithContext(ctx).Create(dep).Error
}

func (s *Store) DeploymentGetByID(ctx context.Context, id string) (*store.Deployment, error) {
	var d store.Deployment
	if err := s.db.WithContext(ctx).First(&d, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) DeploymentList(ctx context.Context) ([]*store.Deployment, error) {
	var deps []*store.Deployment
	if err := s.db.WithContext(ctx).Find(&deps).Error; err != nil {
		return nil, err
	}
	return deps, nil
}

func (s *Store) DeploymentListByArtifact(ctx context.Context, artifactID string) ([]*store.Deployment, error) {
	var deps []*store.Deployment
	if err := s.db.WithContext(ctx).Where("artifact_id = ?", artifactID).Find(&deps).Error; err != nil {
		return nil, err
	}
	return deps, nil
}

func (s *Store) DeploymentUpdate(ctx context.Context, dep *store.Deployment) error {
	return s.db.WithContext(ctx).Save(dep).Error
}

func (s *Store) DeploymentDelete(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).Delete(&store.Deployment{}, "id = ?", id).Error
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return fmt.Errorf("getting underlying DB: %w", err)
	}
	return sqlDB.Close()
}
