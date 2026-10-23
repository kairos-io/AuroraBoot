package gorm

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/kairos-io/AuroraBoot/pkg/store"
)

const defaultBatchListLimit = 50

// BatchCreate writes the batch and its node rows in one transaction.
func (s *Store) BatchCreate(ctx context.Context, b *store.CommandBatch, nodes []*store.BatchNode) error {
	for _, n := range nodes {
		if n.State != store.BatchNodeWaiting && n.State != "" {
			return fmt.Errorf("batch node %s must start waiting, got %q", n.NodeID, n.State)
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		b.Phase = store.BatchRunning
		// Nodes are written through their own table below, with the batch ID
		// set from the batch.
		rec := *b
		rec.Nodes = nil
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		b.CreatedAt = rec.CreatedAt
		for _, n := range nodes {
			n.BatchID = b.ID
			n.State = store.BatchNodeWaiting
		}
		if len(nodes) > 0 {
			if err := tx.Create(&nodes).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// BatchGet returns a batch with its nodes ordered by position and its counts.
func (s *Store) BatchGet(ctx context.Context, id string) (*store.CommandBatch, error) {
	var b store.CommandBatch
	if err := s.db.WithContext(ctx).First(&b, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, store.ErrBatchNotFound
		}
		return nil, err
	}
	var nodes []*store.BatchNode
	if err := s.db.WithContext(ctx).Where("batch_id = ?", id).Order("position ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	b.Nodes = nodes
	b.Counts = map[string]int{}
	for _, n := range nodes {
		b.Counts[n.State]++
	}
	return &b, nil
}

// BatchList returns batches newest first, without nodes, and the total number
// matching the filter.
func (s *Store) BatchList(ctx context.Context, f store.BatchFilter) ([]*store.CommandBatch, int, error) {
	q := s.db.WithContext(ctx).Model(&store.CommandBatch{})
	if f.GroupID != "" {
		q = q.Where("group_id = ?", f.GroupID)
	}
	if f.NodeID != "" {
		q = q.Where("id IN (SELECT batch_id FROM command_batch_nodes WHERE node_id = ?)", f.NodeID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 {
		limit = defaultBatchListLimit
	}
	var batches []*store.CommandBatch
	if err := q.Order("created_at DESC, id DESC").Limit(limit).Offset(f.Offset).Find(&batches).Error; err != nil {
		return nil, 0, err
	}
	if err := s.fillBatchCounts(ctx, batches); err != nil {
		return nil, 0, err
	}
	return batches, int(total), nil
}

func (s *Store) fillBatchCounts(ctx context.Context, batches []*store.CommandBatch) error {
	byID := make(map[string]*store.CommandBatch, len(batches))
	ids := make([]string, 0, len(batches))
	for _, b := range batches {
		b.Counts = map[string]int{}
		byID[b.ID] = b
		ids = append(ids, b.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	var rows []struct {
		BatchID string
		State   string
		N       int
	}
	if err := s.db.WithContext(ctx).Model(&store.BatchNode{}).
		Select("batch_id, state, COUNT(*) AS n").
		Where("batch_id IN ?", ids).
		Group("batch_id, state").
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		byID[r.BatchID].Counts[r.State] = r.N
	}
	return nil
}

// BatchBusyNodes returns the IDs among nodeIDs that have a row in a non-final state.
func (s *Store) BatchBusyNodes(ctx context.Context, nodeIDs []string) ([]string, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	var busy []string
	err := s.db.WithContext(ctx).Model(&store.BatchNode{}).
		Where("node_id IN ?", nodeIDs).
		Where("state NOT IN ?", []string{
			store.BatchNodeDone, store.BatchNodeFailed, store.BatchNodeTimedOut,
			store.BatchNodeSkipped, store.BatchNodeCanceled,
		}).
		Distinct().Pluck("node_id", &busy).Error
	return busy, err
}

// BatchAdvance brings one batch up to date at now, in one transaction: it
// settles node rows from their nodes and commands, starts waiting online nodes
// in free slots, skips the waiting nodes once only offline ones are left, and
// finishes the batch when every row is final. It returns the commands it
// created, for the caller to push once the transaction has committed.
//
// The batch row is locked first, so two advances of the same batch run one
// after the other and the second sees the slots the first one filled.
func (s *Store) BatchAdvance(ctx context.Context, batchID string, now time.Time) ([]*store.NodeCommand, error) {
	var created []*store.NodeCommand
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		b, err := lockBatch(tx, batchID)
		if err != nil {
			return err
		}
		var rows []*store.BatchNode
		if err := tx.Where("batch_id = ?", batchID).Order("position ASC").Find(&rows).Error; err != nil {
			return err
		}
		a := &batchAdvance{batch: b, rows: rows, now: now, changed: map[*store.BatchNode]bool{}}
		if err := a.load(tx); err != nil {
			return err
		}
		a.cancelDeletedNodes()
		a.reconcileRunning()
		if b.Phase == store.BatchRunning {
			cmds := a.fillSlots()
			if err := createCommands(tx, cmds); err != nil {
				return err
			}
			created = cmds
			a.skipOffline()
		}
		for _, row := range rows {
			if !a.changed[row] {
				continue
			}
			if err := tx.Save(row).Error; err != nil {
				return err
			}
		}
		return a.finish(tx)
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// lockBatch reads the batch row under a row lock. SQLite has no row locks;
// the transaction's first write takes the database's single write lock there.
func lockBatch(tx *gorm.DB, id string) (*store.CommandBatch, error) {
	q := tx
	if tx.Dialector.Name() != "sqlite" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var b store.CommandBatch
	if err := q.First(&b, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, store.ErrBatchNotFound
		}
		return nil, err
	}
	return &b, nil
}

// batchAdvance holds what one BatchAdvance read and the rows it changed.
type batchAdvance struct {
	batch *store.CommandBatch
	rows  []*store.BatchNode
	now   time.Time
	// nodePhase maps each existing node of the batch to its phase.
	nodePhase map[string]string
	// commands maps the command ID of each running row to its command.
	commands map[string]*store.NodeCommand
	changed  map[*store.BatchNode]bool
}

func (a *batchAdvance) load(tx *gorm.DB) error {
	nodeIDs := make([]string, 0, len(a.rows))
	var commandIDs []string
	for _, row := range a.rows {
		nodeIDs = append(nodeIDs, row.NodeID)
		if row.State == store.BatchNodeRunning && row.CommandID != nil {
			commandIDs = append(commandIDs, *row.CommandID)
		}
	}
	a.nodePhase = map[string]string{}
	if len(nodeIDs) > 0 {
		var nodes []struct {
			ID    string
			Phase string
		}
		if err := tx.Model(&store.ManagedNode{}).Select("id, phase").
			Where("id IN ?", nodeIDs).Scan(&nodes).Error; err != nil {
			return err
		}
		for _, n := range nodes {
			a.nodePhase[n.ID] = n.Phase
		}
	}
	a.commands = map[string]*store.NodeCommand{}
	if len(commandIDs) > 0 {
		var cmds []*store.NodeCommand
		if err := tx.Where("id IN ?", commandIDs).Find(&cmds).Error; err != nil {
			return err
		}
		for _, c := range cmds {
			a.commands[c.ID] = c
		}
	}
	return nil
}

func (a *batchAdvance) settle(row *store.BatchNode, state, message string) {
	row.State = state
	row.Message = message
	now := a.now
	row.FinishedAt = &now
	a.changed[row] = true
}

func (a *batchAdvance) cancelDeletedNodes() {
	for _, row := range a.rows {
		if store.IsFinalBatchNodeState(row.State) {
			continue
		}
		if _, ok := a.nodePhase[row.NodeID]; !ok {
			a.settle(row, store.BatchNodeCanceled, "node deleted")
		}
	}
}

// reconcileRunning settles each running row from its command. A rebooting row
// waits on its node coming back, which its command cannot report.
func (a *batchAdvance) reconcileRunning() {
	for _, row := range a.rows {
		if row.State != store.BatchNodeRunning {
			continue
		}
		var cmd *store.NodeCommand
		if row.CommandID != nil {
			cmd = a.commands[*row.CommandID]
		}
		if cmd == nil {
			a.settle(row, store.BatchNodeCanceled, "command deleted")
			continue
		}
		switch cmd.Phase {
		case store.CommandFailed:
			a.settle(row, store.BatchNodeFailed, cmd.Result)
		case store.CommandExpired:
			a.settle(row, store.BatchNodeCanceled, "command expired")
		case store.CommandCanceled:
			a.settle(row, store.BatchNodeCanceled, "command canceled")
		case store.CommandCompleted:
			if !store.RebootsNode(cmd.Command) {
				a.settle(row, store.BatchNodeDone, "")
				continue
			}
			since := a.now
			if cmd.CompletedAt != nil {
				since = *cmd.CompletedAt
			}
			deadline := since.Add(a.rebootTimeout())
			row.State = store.BatchNodeRebooting
			row.RebootingSince = &since
			row.Deadline = &deadline
			a.changed[row] = true
		}
	}
}

func (a *batchAdvance) rebootTimeout() time.Duration {
	seconds := a.batch.RebootTimeoutSeconds
	if seconds <= 0 {
		seconds = store.DefaultRebootTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

// holding counts the rows that hold a slot.
func (a *batchAdvance) holding() int {
	n := 0
	for _, row := range a.rows {
		if row.State == store.BatchNodeRunning || row.State == store.BatchNodeRebooting {
			n++
		}
	}
	return n
}

// fillSlots moves the first waiting rows whose node is online to running, as
// many as there are free slots, and returns the commands to create for them.
// Offline nodes stay waiting so a later slot can still start them.
func (a *batchAdvance) fillSlots() []*store.NodeCommand {
	free := a.batch.Concurrency - a.holding()
	if a.batch.Concurrency == 0 {
		free = len(a.rows)
	}
	var cmds []*store.NodeCommand
	for _, row := range a.rows {
		if free <= 0 {
			break
		}
		if row.State != store.BatchNodeWaiting || a.nodePhase[row.NodeID] != store.PhaseOnline {
			continue
		}
		cmd := &store.NodeCommand{
			ID:            uuid.New().String(),
			ManagedNodeID: row.NodeID,
			Command:       a.batch.Command,
			Args:          maps.Clone(a.batch.Args),
			Phase:         store.CommandPending,
			BatchID:       a.batch.ID,
		}
		cmds = append(cmds, cmd)
		row.State = store.BatchNodeRunning
		row.CommandID = &cmd.ID
		now := a.now
		row.StartedAt = &now
		a.changed[row] = true
		free--
	}
	return cmds
}

// skipOffline settles the waiting rows as skipped when no row holds a slot,
// which after fillSlots means every waiting node is offline.
func (a *batchAdvance) skipOffline() {
	if a.holding() > 0 {
		return
	}
	for _, row := range a.rows {
		if row.State == store.BatchNodeWaiting {
			a.settle(row, store.BatchNodeSkipped, "offline when the batch reached it")
		}
	}
}

// finish sets the outcome of a running batch and its finish time once every
// row is final. A stopped batch keeps its phase and gets its finish time when
// its last running node settles.
func (a *batchAdvance) finish(tx *gorm.DB) error {
	failed := false
	for _, row := range a.rows {
		if !store.IsFinalBatchNodeState(row.State) {
			return nil
		}
		if row.State == store.BatchNodeFailed || row.State == store.BatchNodeTimedOut {
			failed = true
		}
	}
	updates := map[string]any{}
	if a.batch.Phase == store.BatchRunning {
		updates["phase"] = store.BatchCompleted
		if failed {
			updates["phase"] = store.BatchFailed
		}
	}
	if a.batch.FinishedAt == nil {
		updates["finished_at"] = a.now
	}
	if len(updates) == 0 {
		return nil
	}
	return tx.Model(&store.CommandBatch{}).Where("id = ?", a.batch.ID).Updates(updates).Error
}
