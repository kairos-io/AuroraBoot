package batches_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/batches"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

var _ = Describe("Scheduler", func() {
	var (
		s         *gormstore.Store
		bs        store.BatchStore
		sched     *batches.Scheduler
		ctx       context.Context
		mu        sync.Mutex
		delivered []*store.NodeCommand
	)

	BeforeEach(func() {
		var err error
		s, err = gormstore.New(filepath.Join(GinkgoT().TempDir(), "sched.db"))
		Expect(err).NotTo(HaveOccurred())
		bs = &gormstore.BatchStoreAdapter{S: s}
		ctx = context.Background()
		delivered = nil
		sched = batches.New(bs, func(_ context.Context, cmd *store.NodeCommand) {
			mu.Lock()
			defer mu.Unlock()
			delivered = append(delivered, cmd)
		})
	})

	AfterEach(func() { Expect(s.Close()).To(Succeed()) })

	deliveredCmds := func() []*store.NodeCommand {
		mu.Lock()
		defer mu.Unlock()
		return append([]*store.NodeCommand(nil), delivered...)
	}

	onlineNodes := func(n int) []*store.ManagedNode {
		nodes := make([]*store.ManagedNode, n)
		for i := range nodes {
			nodes[i] = &store.ManagedNode{MachineID: uuid.NewString(), Hostname: uuid.NewString()}
			Expect(s.Register(ctx, nodes[i])).To(Succeed())
			Expect(s.UpdatePhase(ctx, nodes[i].ID, store.PhaseOnline)).To(Succeed())
		}
		return nodes
	}

	request := func(command string, concurrency int, nodes ...*store.ManagedNode) batches.StartRequest {
		return batches.StartRequest{
			Command:              command,
			Args:                 map[string]string{"k": "v"},
			Nodes:                nodes,
			Selection:            "test",
			Concurrency:          concurrency,
			StopOnFailure:        true,
			RebootTimeoutSeconds: 600,
		}
	}

	It("creates the batch and delivers the first slot's commands", func() {
		nodes := onlineNodes(3)
		b, err := sched.Start(ctx, request(store.CmdExec, 2, nodes...))
		Expect(err).NotTo(HaveOccurred())
		Expect(b.ID).NotTo(BeEmpty())
		Expect(b.Phase).To(Equal(store.BatchRunning))
		Expect(b.Nodes).To(HaveLen(3))
		Expect(b.Counts).To(Equal(map[string]int{store.BatchNodeRunning: 2, store.BatchNodeWaiting: 1}))

		cmds := deliveredCmds()
		Expect(cmds).To(HaveLen(2))
		Expect(cmds[0].ManagedNodeID).To(Equal(nodes[0].ID))
		Expect(cmds[1].ManagedNodeID).To(Equal(nodes[1].ID))
	})

	It("gives a node listed twice one row", func() {
		nodes := onlineNodes(1)
		b, err := sched.Start(ctx, request(store.CmdExec, 0, nodes[0], nodes[0]))
		Expect(err).NotTo(HaveOccurred())
		Expect(b.Nodes).To(HaveLen(1))
		Expect(deliveredCmds()).To(HaveLen(1))
	})

	It("refuses to start when a node is in a batch that has not finished with it", func() {
		nodes := onlineNodes(2)
		_, err := sched.Start(ctx, request(store.CmdExec, 1, nodes[0]))
		Expect(err).NotTo(HaveOccurred())

		_, err = sched.Start(ctx, request(store.CmdExec, 1, nodes[0], nodes[1]))
		var busy *batches.ErrNodesBusy
		Expect(errors.As(err, &busy)).To(BeTrue())
		Expect(busy.NodeIDs).To(Equal([]string{nodes[0].ID}))

		list, total, err := bs.List(ctx, store.BatchFilter{})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(list).To(HaveLen(1))
	})

	It("returns ErrNoNodes for an empty node list", func() {
		_, err := sched.Start(ctx, request(store.CmdExec, 1))
		Expect(err).To(MatchError(batches.ErrNoNodes))
	})

	It("delivers the next node's command after the previous one completed", func() {
		nodes := onlineNodes(2)
		b, err := sched.Start(ctx, request(store.CmdExec, 1, nodes...))
		Expect(err).NotTo(HaveOccurred())
		first := deliveredCmds()
		Expect(first).To(HaveLen(1))

		Expect(s.UpdateStatus(ctx, first[0].ID, store.CommandCompleted, "")).To(Succeed())
		sched.Advance(ctx, b.ID)

		cmds := deliveredCmds()
		Expect(cmds).To(HaveLen(2))
		Expect(cmds[1].ManagedNodeID).To(Equal(nodes[1].ID))
	})

	It("advances every active batch on a tick", func() {
		nodes := onlineNodes(4)
		a, err := sched.Start(ctx, request(store.CmdExec, 1, nodes[0], nodes[1]))
		Expect(err).NotTo(HaveOccurred())
		c, err := sched.Start(ctx, request(store.CmdExec, 1, nodes[2], nodes[3]))
		Expect(err).NotTo(HaveOccurred())
		Expect(deliveredCmds()).To(HaveLen(2))

		for _, cmd := range deliveredCmds() {
			Expect(s.UpdateStatus(ctx, cmd.ID, store.CommandCompleted, "")).To(Succeed())
		}
		sched.Tick(ctx)

		cmds := deliveredCmds()
		Expect(cmds).To(HaveLen(4))
		Expect([]string{cmds[2].BatchID, cmds[3].BatchID}).To(ConsistOf(a.ID, c.ID))
	})

	rebootingBatch := func() (*store.CommandBatch, *store.ManagedNode) {
		nodes := onlineNodes(1)
		b, err := sched.Start(ctx, request(store.CmdUpgrade, 1, nodes[0]))
		Expect(err).NotTo(HaveOccurred())
		cmds := deliveredCmds()
		Expect(cmds).To(HaveLen(1))
		Expect(s.UpdateStatus(ctx, cmds[0].ID, store.CommandCompleted, "")).To(Succeed())
		sched.Tick(ctx)
		got, err := bs.Get(ctx, b.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Nodes[0].State).To(Equal(store.BatchNodeRebooting))
		return got, nodes[0]
	}

	It("times out a rebooting node on a tick after its deadline", func() {
		b, _ := rebootingBatch()
		sched.SetClock(func() time.Time { return b.Nodes[0].Deadline.Add(time.Minute) })

		sched.Tick(ctx)

		got, err := bs.Get(ctx, b.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Nodes[0].State).To(Equal(store.BatchNodeTimedOut))
	})

	It("postpones expired deadlines when it starts running", func() {
		b, _ := rebootingBatch()
		after := b.Nodes[0].Deadline.Add(time.Hour)
		sched.SetClock(func() time.Time { return after })

		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			sched.Run(runCtx, 10*time.Millisecond)
		}()
		Eventually(func() bool {
			got, err := bs.Get(ctx, b.ID)
			Expect(err).NotTo(HaveOccurred())
			return got.Nodes[0].Deadline.After(after)
		}, 5*time.Second, 10*time.Millisecond).Should(BeTrue())
		cancel()
		Eventually(done).Should(BeClosed())

		got, err := bs.Get(ctx, b.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Nodes[0].State).To(Equal(store.BatchNodeRebooting))
		Expect(got.Nodes[0].Deadline).To(HaveValue(BeTemporally("==", after.Add(600*time.Second))))
	})
})
