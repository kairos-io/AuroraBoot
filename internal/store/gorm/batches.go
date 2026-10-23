package gorm

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

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
