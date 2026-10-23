package gorm_test

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// SQLite runs one writer at a time, so two advances can only overlap on
// PostgreSQL.
var _ = Describe("Gorm batch store on PostgreSQL", func() {
	var (
		s     *gormstore.Store
		ctx   context.Context
		meet  *rendezvous
		armed atomic.Bool
	)

	BeforeEach(func() {
		ctx = context.Background()
		s = newPostgresStore()

		// Both advances read the node rows of the batch before they decide
		// how many slots are free. Holding both there makes them overlap at
		// the point where one has to see the other's starts.
		meet = newRendezvous()
		armed.Store(false)
		hold := func(db *gorm.DB) {
			if armed.Load() && !db.DryRun && db.Statement.Table == "command_batch_nodes" {
				meet.arrive()
			}
		}
		Expect(s.UnsafeDB().Callback().Query().Before("gorm:query").Register("test:rendezvous", hold)).To(Succeed())
	})

	It("never starts more nodes than the concurrency when two advances run at once", func() {
		nodes := make([]*store.BatchNode, 6)
		for i := range nodes {
			n := &store.ManagedNode{MachineID: uuid.NewString()}
			Expect(s.Register(ctx, n)).To(Succeed())
			Expect(s.UpdatePhase(ctx, n.ID, store.PhaseOnline)).To(Succeed())
			nodes[i] = &store.BatchNode{NodeID: n.ID, Position: i, State: store.BatchNodeWaiting}
		}
		b := &store.CommandBatch{ID: uuid.NewString(), Command: store.CmdExec, Concurrency: 2}
		Expect(s.BatchCreate(ctx, b, nodes)).To(Succeed())

		armed.Store(true)
		var wg sync.WaitGroup
		started := make([][]*store.NodeCommand, 2)
		errs := make([]error, 2)
		for i := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				started[i], errs[i] = s.BatchAdvance(ctx, b.ID, time.Now())
			}()
		}
		wg.Wait()
		armed.Store(false)
		Expect(errs).To(HaveEach(Succeed()))

		Expect(len(started[0]) + len(started[1])).To(Equal(2))
		var running, commands int64
		Expect(s.UnsafeDB().Model(&store.BatchNode{}).
			Where("batch_id = ? AND state = ?", b.ID, store.BatchNodeRunning).
			Count(&running).Error).To(Succeed())
		Expect(running).To(Equal(int64(2)))
		Expect(s.UnsafeDB().Model(&store.NodeCommand{}).
			Where("batch_id = ?", b.ID).
			Count(&commands).Error).To(Succeed())
		Expect(commands).To(Equal(int64(2)))
	})
})
