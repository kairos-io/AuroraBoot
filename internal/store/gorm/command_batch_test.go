package gorm_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

var _ = Describe("Gorm Store command batches", func() {
	var (
		s   *gormstore.Store
		ctx context.Context
	)

	// queue inserts one command and returns the id the store minted for it.
	// CommandCreate owns both the id and the phase: it overwrites whatever the
	// caller set with a fresh UUID and CommandPending. A spec that needs a
	// later phase therefore has to move the row after the insert, the same way
	// a real caller does.
	queue := func(nodeID, batchID, phase string) string {
		cmd := &store.NodeCommand{
			ManagedNodeID: nodeID,
			Command:       store.CmdUpgrade,
			BatchID:       batchID,
			FailFast:      batchID != "",
		}
		Expect(s.CommandCreate(ctx, cmd)).To(Succeed())
		Expect(cmd.ID).NotTo(BeEmpty())
		if phase != store.CommandPending {
			Expect(s.UpdateStatus(ctx, cmd.ID, phase, "")).To(Succeed())
		}
		return cmd.ID
	}

	phaseOf := func(id string) string {
		got, err := s.CommandGetByID(ctx, id)
		Expect(err).NotTo(HaveOccurred())
		return got.Phase
	}

	BeforeEach(func() {
		var err error
		s, err = gormstore.New(filepath.Join(GinkgoT().TempDir(), "batch.db"))
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() { Expect(s.Close()).To(Succeed()) })

	It("round-trips the batch id and the fail-fast flag", func() {
		id := queue("n1", "batch-a", store.CommandPending)

		got, err := s.CommandGetByID(ctx, id)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.BatchID).To(Equal("batch-a"))
		Expect(got.FailFast).To(BeTrue())
	})

	It("ListByBatch returns only the commands of that batch", func() {
		c1 := queue("n1", "batch-a", store.CommandPending)
		c2 := queue("n2", "batch-a", store.CommandPending)
		queue("n3", "batch-b", store.CommandPending)
		queue("n4", "", store.CommandPending)

		got, err := s.ListByBatch(ctx, "batch-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(HaveLen(2))
		ids := []string{got[0].ID, got[1].ID}
		Expect(ids).To(ConsistOf(c1, c2))
	})

	// An empty batch id is what an unbatched, single-node command carries. A
	// query on it must not match every such command, or one failed single-node
	// upgrade would cancel every other queued single-node command.
	It("ListByBatch on an empty id matches nothing", func() {
		queue("n1", "", store.CommandPending)

		got, err := s.ListByBatch(ctx, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeEmpty())
	})

	It("CancelPendingInBatch moves only the Pending rows of the batch", func() {
		pending1 := queue("n1", "batch-a", store.CommandPending)
		pending2 := queue("n2", "batch-a", store.CommandPending)
		delivered := queue("n3", "batch-a", store.CommandDelivered)
		running := queue("n4", "batch-a", store.CommandRunning)
		failed := queue("n5", "batch-a", store.CommandFailed)
		otherBatch := queue("n6", "batch-b", store.CommandPending)
		unbatched := queue("n7", "", store.CommandPending)

		n, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped: node n5 failed")
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(2))

		Expect(phaseOf(pending1)).To(Equal(store.CommandCanceled))
		Expect(phaseOf(pending2)).To(Equal(store.CommandCanceled))
		Expect(phaseOf(delivered)).To(Equal(store.CommandDelivered))
		Expect(phaseOf(running)).To(Equal(store.CommandRunning))
		Expect(phaseOf(failed)).To(Equal(store.CommandFailed))
		Expect(phaseOf(otherBatch)).To(Equal(store.CommandPending))
		Expect(phaseOf(unbatched)).To(Equal(store.CommandPending))
	})

	It("CancelPendingInBatch records the reason and stamps the completion time", func() {
		id := queue("n1", "batch-a", store.CommandPending)

		_, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped: node n9 failed")
		Expect(err).NotTo(HaveOccurred())

		got, err := s.CommandGetByID(ctx, id)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Result).To(Equal("stopped: node n9 failed"))
		Expect(got.CompletedAt).NotTo(BeNil())
	})

	It("CancelPendingInBatch on an empty batch id cancels nothing", func() {
		id := queue("n1", "", store.CommandPending)

		n, err := s.CancelPendingInBatch(ctx, "", "should not happen")
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(BeZero())
		Expect(phaseOf(id)).To(Equal(store.CommandPending))
	})

	// The cancel and ClaimForDelivery are the same compare-and-set on the same
	// row: whichever lands first leaves no Pending row for the other, so a
	// command is never both canceled and pushed to a node.
	It("a cancel and a delivery claim cannot both win the same command", func() {
		id := queue("n1", "batch-a", store.CommandPending)

		n, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(1))

		claimed, err := s.ClaimForDelivery(ctx, id)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeFalse())
		Expect(phaseOf(id)).To(Equal(store.CommandCanceled))
	})

	Describe("CreateBatch", func() {
		register := func(machineID string) *store.ManagedNode {
			n := &store.ManagedNode{MachineID: machineID}
			Expect(s.Register(ctx, n)).To(Succeed())
			return n
		}

		node := func(id string) *store.ManagedNode {
			got, err := s.NodeGetByID(ctx, id)
			Expect(err).NotTo(HaveOccurred())
			return got
		}

		batchOf := func(command string, ids []string, nodes ...*store.ManagedNode) []*store.NodeCommand {
			cmds := make([]*store.NodeCommand, 0, len(nodes))
			for i, n := range nodes {
				cmds = append(cmds, &store.NodeCommand{
					ID:            ids[i],
					ManagedNodeID: n.ID,
					Command:       command,
					Phase:         store.CommandPending,
					BatchID:       "batch-a",
					FailFast:      true,
				})
			}
			return cmds
		}

		It("writes every row with the ids the caller gave it", func() {
			n1, n2 := register("m1"), register("m2")

			Expect(s.CreateBatch(ctx, batchOf(store.CmdUpgrade, []string{"c1", "c2"}, n1, n2))).To(Succeed())

			got, err := s.ListByBatch(ctx, "batch-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(HaveLen(2))
			Expect([]string{got[0].ID, got[1].ID}).To(ConsistOf("c1", "c2"))
			for _, cmd := range got {
				Expect(cmd.Phase).To(Equal(store.CommandPending))
				Expect(cmd.FailFast).To(BeTrue())
			}
		})

		It("marks every upgrade target pending in the same write", func() {
			n1, n2, other := register("m1"), register("m2"), register("m3")

			Expect(s.CreateBatch(ctx, batchOf(store.CmdUpgrade, []string{"c1", "c2"}, n1, n2))).To(Succeed())

			for _, id := range []string{n1.ID, n2.ID} {
				got := node(id)
				Expect(got.UpgradeState).To(Equal(store.UpgradeStatePending))
				Expect(got.UpgradeRequestedAt).NotTo(BeNil())
				Expect(got.ResetState).To(BeEmpty())
			}
			Expect(node(other.ID).UpgradeState).To(BeEmpty())
		})

		It("marks every reset target pending in the same write", func() {
			n1, n2 := register("m1"), register("m2")

			Expect(s.CreateBatch(ctx, batchOf(store.CmdReset, []string{"c1", "c2"}, n1, n2))).To(Succeed())

			for _, id := range []string{n1.ID, n2.ID} {
				got := node(id)
				Expect(got.ResetState).To(Equal(store.ResetStatePending))
				Expect(got.ResetRequestedAt).NotTo(BeNil())
				Expect(got.UpgradeState).To(BeEmpty())
			}
		})

		It("leaves the lifecycle alone for a command that does not reboot the node", func() {
			n1 := register("m1")

			Expect(s.CreateBatch(ctx, batchOf("reboot", []string{"c1"}, n1))).To(Succeed())

			got := node(n1.ID)
			Expect(got.UpgradeState).To(BeEmpty())
			Expect(got.ResetState).To(BeEmpty())
		})

		// An agent may claim any committed Pending row, so a batch that is
		// half written can be half delivered. A failure part way through has
		// to leave nothing behind.
		It("writes nothing when one row of the batch cannot be stored", func() {
			n1, n2 := register("m1"), register("m2")

			err := s.CreateBatch(ctx, batchOf(store.CmdUpgrade, []string{"dup", "dup"}, n1, n2))
			Expect(err).To(HaveOccurred())

			got, err := s.ListByBatch(ctx, "batch-a")
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(BeEmpty())
			for _, id := range []string{n1.ID, n2.ID} {
				Expect(node(id).UpgradeState).To(BeEmpty())
				Expect(node(id).UpgradeRequestedAt).To(BeNil())
			}
		})
	})

	// A canceled command never runs, so the pending lifecycle state its batch
	// set has nothing left to resolve it. Left in place, the next unrelated
	// active boot would report the upgrade or reset done.
	Describe("CancelPendingInBatch lifecycle", func() {
		register := func(machineID string) *store.ManagedNode {
			n := &store.ManagedNode{MachineID: machineID}
			Expect(s.Register(ctx, n)).To(Succeed())
			return n
		}

		node := func(id string) *store.ManagedNode {
			got, err := s.NodeGetByID(ctx, id)
			Expect(err).NotTo(HaveOccurred())
			return got
		}

		// publish writes one command per node as a fail-fast batch and returns
		// the ids in node order.
		publish := func(batchID, command string, nodes ...*store.ManagedNode) []string {
			cmds := make([]*store.NodeCommand, 0, len(nodes))
			ids := make([]string, 0, len(nodes))
			for _, n := range nodes {
				cmd := &store.NodeCommand{
					ManagedNodeID: n.ID,
					Command:       command,
					BatchID:       batchID,
					FailFast:      true,
				}
				cmds = append(cmds, cmd)
			}
			Expect(s.CreateBatch(ctx, cmds)).To(Succeed())
			for _, cmd := range cmds {
				ids = append(ids, cmd.ID)
			}
			return ids
		}

		It("clears the pending upgrade of a canceled target", func() {
			n1, n2 := register("m1"), register("m2")
			ids := publish("batch-a", store.CmdUpgrade, n1, n2)
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			n, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(1))

			got := node(n2.ID)
			Expect(got.UpgradeState).To(BeEmpty())
			Expect(got.UpgradeRequestedAt).To(BeNil())
			Expect(got.LastUpgrade).To(BeNil())
		})

		It("keeps the pending upgrade of a node that has another live upgrade", func() {
			n1, n2 := register("m1"), register("m2")
			ids := publish("batch-a", store.CmdUpgrade, n1, n2)
			other := publish("batch-b", store.CmdUpgrade, n2)
			Expect(s.UpdateStatus(ctx, other[0], store.CommandRunning, "")).To(Succeed())
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			_, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			Expect(err).NotTo(HaveOccurred())

			got := node(n2.ID)
			Expect(got.UpgradeState).To(Equal(store.UpgradeStatePending))
			Expect(got.UpgradeRequestedAt).NotTo(BeNil())
		})

		It("leaves an upgrade state that is no longer pending alone", func() {
			n1, n2 := register("m1"), register("m2")
			ids := publish("batch-a", store.CmdUpgrade, n1, n2)
			_, err := s.AdvanceUpgrade(ctx, n2.ID, []string{store.UpgradeStatePending}, store.UpgradeStateDone, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			_, err = s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			Expect(err).NotTo(HaveOccurred())

			got := node(n2.ID)
			Expect(got.UpgradeState).To(Equal(store.UpgradeStateDone))
			Expect(got.UpgradeRequestedAt).NotTo(BeNil())
			Expect(got.LastUpgrade).NotTo(BeNil())
		})

		It("clears the pending reset of a canceled target", func() {
			n1, n2 := register("m1"), register("m2")
			ids := publish("batch-a", store.CmdReset, n1, n2)
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			_, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			Expect(err).NotTo(HaveOccurred())

			got := node(n2.ID)
			Expect(got.ResetState).To(BeEmpty())
			Expect(got.ResetRequestedAt).To(BeNil())
			Expect(got.LastReset).To(BeNil())
		})

		It("keeps the pending reset of a node that has another live reset", func() {
			n1, n2 := register("m1"), register("m2")
			ids := publish("batch-a", store.CmdReset, n1, n2)
			publish("batch-b", store.CmdReset, n2)
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			_, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			Expect(err).NotTo(HaveOccurred())

			Expect(node(n2.ID).ResetState).To(Equal(store.ResetStatePending))
		})

		// in-progress means the node has already rebooted into the reset, so
		// the operation is real whatever happened to the command row.
		It("leaves a reset that is already in progress alone", func() {
			n1, n2 := register("m1"), register("m2")
			ids := publish("batch-a", store.CmdReset, n1, n2)
			_, err := s.AdvanceReset(ctx, n2.ID, []string{store.ResetStatePending}, store.ResetStateInProgress, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			_, err = s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			Expect(err).NotTo(HaveOccurred())

			got := node(n2.ID)
			Expect(got.ResetState).To(Equal(store.ResetStateInProgress))
			Expect(got.ResetRequestedAt).NotTo(BeNil())
		})

		It("leaves the delivered rows of the batch and their nodes alone", func() {
			n1, n2, n3 := register("m1"), register("m2"), register("m3")
			ids := publish("batch-a", store.CmdUpgrade, n1, n2, n3)
			claimed, err := s.ClaimForDelivery(ctx, ids[1])
			Expect(err).NotTo(HaveOccurred())
			Expect(claimed).To(BeTrue())
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			n, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(1))

			Expect(phaseOf(ids[1])).To(Equal(store.CommandDelivered))
			Expect(node(n2.ID).UpgradeState).To(Equal(store.UpgradeStatePending))
			Expect(phaseOf(ids[2])).To(Equal(store.CommandCanceled))
			Expect(node(n3.ID).UpgradeState).To(BeEmpty())
		})
	})

	It("CommandDeleteTerminal clears canceled commands with the rest of the history", func() {
		queue("n1", "batch-a", store.CommandCompleted)
		queue("n1", "batch-a", store.CommandCanceled)
		stillPending := queue("n1", "batch-a", store.CommandPending)

		Expect(s.CommandDeleteTerminal(ctx, "n1")).To(Succeed())

		left, err := s.ListByNode(ctx, "n1")
		Expect(err).NotTo(HaveOccurred())
		Expect(left).To(HaveLen(1))
		Expect(left[0].ID).To(Equal(stillPending))
	})
})
