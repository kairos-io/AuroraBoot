package gorm_test

import (
	"context"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

var _ = Describe("Gorm batch advance", func() {
	var (
		s   *gormstore.Store
		ctx context.Context
	)

	BeforeEach(func() {
		var err error
		s, err = gormstore.New(filepath.Join(GinkgoT().TempDir(), "advance.db"))
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() { Expect(s.Close()).To(Succeed()) })

	register := func(n int) []*store.ManagedNode {
		nodes := make([]*store.ManagedNode, n)
		for i := range nodes {
			nodes[i] = &store.ManagedNode{MachineID: uuid.NewString(), Hostname: uuid.NewString()}
			Expect(s.Register(ctx, nodes[i])).To(Succeed())
		}
		return nodes
	}

	online := func(nodes ...*store.ManagedNode) {
		for _, n := range nodes {
			Expect(s.UpdatePhase(ctx, n.ID, store.PhaseOnline)).To(Succeed())
		}
	}

	offline := func(nodes ...*store.ManagedNode) {
		for _, n := range nodes {
			Expect(s.UpdatePhase(ctx, n.ID, store.PhaseOffline)).To(Succeed())
		}
	}

	newBatch := func(nodes []*store.ManagedNode, command string, concurrency int) string {
		b := &store.CommandBatch{
			ID:                   uuid.NewString(),
			Command:              command,
			Args:                 map[string]string{"k": "v"},
			Concurrency:          concurrency,
			StopOnFailure:        false,
			RebootTimeoutSeconds: store.DefaultRebootTimeoutSeconds,
		}
		rows := make([]*store.BatchNode, len(nodes))
		for i, n := range nodes {
			rows[i] = &store.BatchNode{NodeID: n.ID, Position: i, Hostname: n.Hostname, State: store.BatchNodeWaiting}
		}
		Expect(s.BatchCreate(ctx, b, rows)).To(Succeed())
		return b.ID
	}

	advance := func(batchID string) []*store.NodeCommand {
		cmds, err := s.BatchAdvance(ctx, batchID, time.Now())
		Expect(err).NotTo(HaveOccurred())
		return cmds
	}

	get := func(batchID string) *store.CommandBatch {
		b, err := s.BatchGet(ctx, batchID)
		Expect(err).NotTo(HaveOccurred())
		return b
	}

	It("starts as many online nodes as the concurrency allows, in order", func() {
		nodes := register(3)
		online(nodes...)
		id := newBatch(nodes, store.CmdExec, 2)

		cmds := advance(id)
		Expect(cmds).To(HaveLen(2))
		Expect([]string{cmds[0].ManagedNodeID, cmds[1].ManagedNodeID}).To(Equal([]string{nodes[0].ID, nodes[1].ID}))
		for _, c := range cmds {
			Expect(c.BatchID).To(Equal(id))
			Expect(c.Command).To(Equal(store.CmdExec))
			Expect(c.Args).To(Equal(map[string]string{"k": "v"}))
			Expect(c.Phase).To(Equal(store.CommandPending))
		}

		b := get(id)
		Expect(b.Nodes[0].State).To(Equal(store.BatchNodeRunning))
		Expect(b.Nodes[0].CommandID).To(HaveValue(Equal(cmds[0].ID)))
		Expect(b.Nodes[0].StartedAt).NotTo(BeNil())
		Expect(b.Nodes[1].State).To(Equal(store.BatchNodeRunning))
		Expect(b.Nodes[1].CommandID).To(HaveValue(Equal(cmds[1].ID)))
		Expect(b.Nodes[2].State).To(Equal(store.BatchNodeWaiting))
		Expect(b.Nodes[2].CommandID).To(BeNil())

		pending, err := s.GetPending(ctx, nodes[0].ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(pending).To(HaveLen(1))
		Expect(pending[0].ID).To(Equal(cmds[0].ID))
	})

	It("starts every node when the concurrency is 0", func() {
		nodes := register(3)
		online(nodes...)
		id := newBatch(nodes, store.CmdExec, 0)

		Expect(advance(id)).To(HaveLen(3))
		Expect(get(id).Counts).To(Equal(map[string]int{store.BatchNodeRunning: 3}))
	})

	It("starts the next node when a running node's command completes", func() {
		nodes := register(2)
		online(nodes...)
		id := newBatch(nodes, store.CmdExec, 1)

		first := advance(id)
		Expect(first).To(HaveLen(1))
		Expect(s.UpdateStatus(ctx, first[0].ID, store.CommandCompleted, "")).To(Succeed())

		second := advance(id)
		Expect(second).To(HaveLen(1))
		Expect(second[0].ManagedNodeID).To(Equal(nodes[1].ID))

		b := get(id)
		Expect(b.Nodes[0].State).To(Equal(store.BatchNodeDone))
		Expect(b.Nodes[0].FinishedAt).NotTo(BeNil())
		Expect(b.Nodes[1].State).To(Equal(store.BatchNodeRunning))
	})

	It("moves a node whose upgrade completed to rebooting with its deadline", func() {
		nodes := register(2)
		online(nodes...)
		id := newBatch(nodes, store.CmdUpgrade, 1)

		cmds := advance(id)
		Expect(cmds).To(HaveLen(1))
		Expect(s.UpdateStatus(ctx, cmds[0].ID, store.CommandCompleted, "")).To(Succeed())
		completedAt := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
		Expect(s.UnsafeDB().Model(&store.NodeCommand{}).Where("id = ?", cmds[0].ID).
			Update("completed_at", completedAt).Error).To(Succeed())

		Expect(advance(id)).To(BeEmpty())

		b := get(id)
		Expect(b.Nodes[0].State).To(Equal(store.BatchNodeRebooting))
		Expect(b.Nodes[0].RebootingSince).To(HaveValue(BeTemporally("==", completedAt)))
		Expect(b.Nodes[0].Deadline).To(HaveValue(BeTemporally("==", completedAt.Add(10*time.Minute))))
		Expect(b.Nodes[0].FinishedAt).To(BeNil())
		Expect(b.Nodes[1].State).To(Equal(store.BatchNodeWaiting))
	})

	It("marks a node failed with its command's result", func() {
		nodes := register(1)
		online(nodes...)
		id := newBatch(nodes, store.CmdExec, 1)

		cmds := advance(id)
		Expect(s.UpdateStatus(ctx, cmds[0].ID, store.CommandFailed, "exit status 1")).To(Succeed())
		advance(id)

		b := get(id)
		Expect(b.Nodes[0].State).To(Equal(store.BatchNodeFailed))
		Expect(b.Nodes[0].Message).To(Equal("exit status 1"))
		Expect(b.Nodes[0].FinishedAt).NotTo(BeNil())
		Expect(b.Phase).To(Equal(store.BatchFailed))
		Expect(b.FinishedAt).NotTo(BeNil())
	})

	It("cancels a running node whose command was deleted", func() {
		nodes := register(1)
		online(nodes...)
		id := newBatch(nodes, store.CmdExec, 1)

		cmds := advance(id)
		Expect(s.CommandDelete(ctx, cmds[0].ID)).To(Succeed())
		advance(id)

		b := get(id)
		Expect(b.Nodes[0].State).To(Equal(store.BatchNodeCanceled))
		Expect(b.Nodes[0].Message).To(Equal("command deleted"))
		Expect(b.Nodes[0].FinishedAt).NotTo(BeNil())
	})

	It("cancels the rows of a node that was deleted", func() {
		nodes := register(2)
		online(nodes...)
		id := newBatch(nodes, store.CmdExec, 1)

		advance(id)
		Expect(s.NodeDelete(ctx, nodes[0].ID)).To(Succeed())
		Expect(s.NodeDelete(ctx, nodes[1].ID)).To(Succeed())
		Expect(advance(id)).To(BeEmpty())

		b := get(id)
		for _, row := range b.Nodes {
			Expect(row.State).To(Equal(store.BatchNodeCanceled))
			Expect(row.Message).To(Equal("node deleted"))
			Expect(row.FinishedAt).NotTo(BeNil())
		}
	})

	It("passes over an offline node and starts the next online one", func() {
		nodes := register(2)
		offline(nodes[0])
		online(nodes[1])
		id := newBatch(nodes, store.CmdExec, 1)

		cmds := advance(id)
		Expect(cmds).To(HaveLen(1))
		Expect(cmds[0].ManagedNodeID).To(Equal(nodes[1].ID))

		b := get(id)
		Expect(b.Nodes[0].State).To(Equal(store.BatchNodeWaiting))
		Expect(b.Nodes[1].State).To(Equal(store.BatchNodeRunning))
	})

	It("skips the waiting nodes once only offline nodes are left", func() {
		nodes := register(2)
		online(nodes[0])
		offline(nodes[1])
		id := newBatch(nodes, store.CmdExec, 1)

		cmds := advance(id)
		Expect(s.UpdateStatus(ctx, cmds[0].ID, store.CommandCompleted, "")).To(Succeed())
		Expect(advance(id)).To(BeEmpty())

		b := get(id)
		Expect(b.Nodes[0].State).To(Equal(store.BatchNodeDone))
		Expect(b.Nodes[1].State).To(Equal(store.BatchNodeSkipped))
		Expect(b.Nodes[1].Message).To(Equal("offline when the batch reached it"))
		Expect(b.Nodes[1].FinishedAt).NotTo(BeNil())
		Expect(b.Phase).To(Equal(store.BatchCompleted))
	})

	It("completes the batch when every node is done", func() {
		nodes := register(2)
		online(nodes...)
		id := newBatch(nodes, store.CmdExec, 0)

		for _, c := range advance(id) {
			Expect(s.UpdateStatus(ctx, c.ID, store.CommandCompleted, "")).To(Succeed())
		}
		advance(id)

		b := get(id)
		Expect(b.Phase).To(Equal(store.BatchCompleted))
		Expect(b.FinishedAt).NotTo(BeNil())
		Expect(b.Counts).To(Equal(map[string]int{store.BatchNodeDone: 2}))
	})

	It("marks the upgrade lifecycle of a node when its command is created", func() {
		nodes := register(1)
		online(nodes...)
		id := newBatch(nodes, store.CmdUpgrade, 1)

		Expect(advance(id)).To(HaveLen(1))
		got, err := s.NodeGetByID(ctx, nodes[0].ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.UpgradeState).To(Equal(store.UpgradeStatePending))
	})

	It("returns ErrBatchNotFound for an unknown batch", func() {
		_, err := s.BatchAdvance(ctx, "missing", time.Now())
		Expect(err).To(MatchError(store.ErrBatchNotFound))
	})
})
