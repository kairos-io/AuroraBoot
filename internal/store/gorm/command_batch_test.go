package gorm_test

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

var _ = Describe("Gorm Store command batches", func() {
	var (
		s      *gormstore.Store
		ctx    context.Context
		dbPath string
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
		dbPath = filepath.Join(GinkgoT().TempDir(), "batch.db")
		s, err = gormstore.New(dbPath)
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

		// A node whose earlier upgrade or reset is still waiting for its
		// reboot keeps the request time of that operation, so the resolver
		// that runs when it re-registers judges the operation that is in
		// flight.
		It("leaves an upgrade that is already pending and its request time alone", func() {
			n1, n2 := register("m1"), register("m2")
			issueCommand(ctx, s, n1.ID, store.CmdUpgrade)
			requested := *node(n1.ID).UpgradeRequestedAt

			Expect(s.CreateBatch(ctx, batchOf(store.CmdUpgrade, []string{"c1", "c2"}, n1, n2))).To(Succeed())

			got := node(n1.ID)
			Expect(got.UpgradeState).To(Equal(store.UpgradeStatePending))
			Expect(*got.UpgradeRequestedAt).To(BeTemporally("==", requested))
			Expect(node(n2.ID).UpgradeState).To(Equal(store.UpgradeStatePending))
		})

		It("marks an upgrade target whose last upgrade finished", func() {
			n1 := register("m1")
			issueCommand(ctx, s, n1.ID, store.CmdUpgrade)
			_, err := s.AdvanceUpgrade(ctx, n1.ID, []string{store.UpgradeStatePending}, store.UpgradeStateDone, true)
			Expect(err).NotTo(HaveOccurred())
			requested := *node(n1.ID).UpgradeRequestedAt

			Expect(s.CreateBatch(ctx, batchOf(store.CmdUpgrade, []string{"c1"}, n1))).To(Succeed())

			got := node(n1.ID)
			Expect(got.UpgradeState).To(Equal(store.UpgradeStatePending))
			Expect(*got.UpgradeRequestedAt).To(BeTemporally(">", requested))
		})

		DescribeTable("leaves a reset that is already in flight and its request time alone",
			func(state string) {
				n1, n2 := register("m1"), register("m2")
				issueCommand(ctx, s, n1.ID, store.CmdReset)
				if state != store.ResetStatePending {
					_, err := s.AdvanceReset(ctx, n1.ID, []string{store.ResetStatePending}, state, false)
					Expect(err).NotTo(HaveOccurred())
				}
				requested := *node(n1.ID).ResetRequestedAt

				Expect(s.CreateBatch(ctx, batchOf(store.CmdReset, []string{"c1", "c2"}, n1, n2))).To(Succeed())

				got := node(n1.ID)
				Expect(got.ResetState).To(Equal(state))
				Expect(*got.ResetRequestedAt).To(BeTemporally("==", requested))
				Expect(node(n2.ID).ResetState).To(Equal(store.ResetStatePending))
			},
			Entry("pending", store.ResetStatePending),
			Entry("in-progress", store.ResetStateInProgress),
		)

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

		// single queues one command outside any batch, as the single-node
		// command handler does, then moves it to phase.
		single := func(n *store.ManagedNode, command, phase string) string {
			id := issueCommand(ctx, s, n.ID, command)
			Expect(s.UpdateStatus(ctx, id, phase, "")).To(Succeed())
			return id
		}

		// A Completed upgrade means the agent staged the image and scheduled
		// the reboot. The node's pending state belongs to that upgrade until
		// the node re-registers after the reboot.
		It("keeps the pending upgrade of a node whose earlier upgrade awaits its reboot", func() {
			n1, n2 := register("m1"), register("m2")
			single(n2, store.CmdUpgrade, store.CommandCompleted)
			requested := *node(n2.ID).UpgradeRequestedAt
			ids := publish("batch-a", store.CmdUpgrade, n1, n2)
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			n, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(1))

			got := node(n2.ID)
			Expect(got.UpgradeState).To(Equal(store.UpgradeStatePending))
			Expect(*got.UpgradeRequestedAt).To(BeTemporally("==", requested))
		})

		It("keeps the pending reset of a node whose earlier reset awaits its reboot", func() {
			n1, n2 := register("m1"), register("m2")
			single(n2, store.CmdReset, store.CommandCompleted)
			requested := *node(n2.ID).ResetRequestedAt
			ids := publish("batch-a", store.CmdReset, n1, n2)
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			n, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(1))

			got := node(n2.ID)
			Expect(got.ResetState).To(Equal(store.ResetStatePending))
			Expect(*got.ResetRequestedAt).To(BeTemporally("==", requested))
		})

		// An upgrade that completed before the current request is history:
		// its reboot has already been resolved, so it owns nothing.
		DescribeTable("clears the pending upgrade when the node's completed upgrade is older than the request",
			func(settle func(n *store.ManagedNode)) {
				n1, n2 := register("m1"), register("m2")
				cmd := &store.NodeCommand{ManagedNodeID: n2.ID, Command: store.CmdUpgrade}
				Expect(s.CommandCreate(ctx, cmd)).To(Succeed())
				Expect(s.UpdateStatus(ctx, cmd.ID, store.CommandCompleted, "")).To(Succeed())
				settle(n2)
				ids := publish("batch-a", store.CmdUpgrade, n1, n2)
				Expect(node(n2.ID).UpgradeState).To(Equal(store.UpgradeStatePending))
				Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

				_, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
				Expect(err).NotTo(HaveOccurred())

				got := node(n2.ID)
				Expect(got.UpgradeState).To(BeEmpty())
				Expect(got.UpgradeRequestedAt).To(BeNil())
			},
			Entry("node never marked", func(*store.ManagedNode) {}),
			Entry("node marked and resolved done", func(n *store.ManagedNode) {
				id := issueCommand(ctx, s, n.ID, store.CmdUpgrade)
				Expect(s.UpdateStatus(ctx, id, store.CommandCompleted, "")).To(Succeed())
				_, err := s.AdvanceUpgrade(ctx, n.ID, []string{store.UpgradeStatePending}, store.UpgradeStateDone, true)
				Expect(err).NotTo(HaveOccurred())
			}),
		)

		It("clears the pending reset when the node's completed reset is older than the request", func() {
			n1, n2 := register("m1"), register("m2")
			single(n2, store.CmdReset, store.CommandCompleted)
			_, err := s.AdvanceReset(ctx, n2.ID, []string{store.ResetStatePending}, store.ResetStateDone, true)
			Expect(err).NotTo(HaveOccurred())
			ids := publish("batch-a", store.CmdReset, n1, n2)
			Expect(node(n2.ID).ResetState).To(Equal(store.ResetStatePending))
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			_, err = s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			Expect(err).NotTo(HaveOccurred())

			got := node(n2.ID)
			Expect(got.ResetState).To(BeEmpty())
			Expect(got.ResetRequestedAt).To(BeNil())
		})

		// Agents claim commands and report status on their own connections
		// while a fail-fast batch is being canceled. Under WAL a transaction
		// that reads first works from a snapshot, and once another connection
		// commits, its first write fails with SQLITE_BUSY_SNAPSHOT without
		// waiting on busy_timeout. The spec commits such a write from a second
		// store on the same file as soon as the cancel runs any query against
		// the database; the cancel has to succeed and cancel every Pending row of the batch.
		It("cancels the batch while another connection commits a write", func() {
			n1, n2, n3 := register("m1"), register("m2"), register("m3")
			ids := publish("batch-a", store.CmdUpgrade, n1, n2)
			unrelated := publish("batch-b", store.CmdUpgrade, n3)
			Expect(s.UpdateStatus(ctx, ids[0], store.CommandFailed, "boom")).To(Succeed())

			agent, err := gormstore.New(dbPath)
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(agent.Close()).To(Succeed()) }()

			armed := false
			var agentErr error
			Expect(s.UnsafeDB().Callback().Query().After("gorm:query").
				Register("test:concurrent_agent_write", func(db *gorm.DB) {
					// GORM renders a subquery by running the query
					// callbacks in DryRun mode; that never reaches the
					// database, so it holds no snapshot.
					if !armed || db.DryRun {
						return
					}
					armed = false
					agentErr = agent.UpdateStatus(ctx, unrelated[0], store.CommandRunning, "")
				})).To(Succeed())

			armed = true
			n, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
			armed = false
			Expect(agentErr).NotTo(HaveOccurred())
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(1))
			Expect(phaseOf(ids[1])).To(Equal(store.CommandCanceled))
			Expect(node(n2.ID).UpgradeState).To(BeEmpty())
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

// A throttled batch holds most of its rows in Queued and releases them one
// slot at a time (kairos-io/kairos#4267, acceptance criterion 2).
var _ = Describe("Gorm Store batch concurrency", func() {
	var (
		s   *gormstore.Store
		ctx context.Context
	)

	BeforeEach(func() {
		var err error
		s, err = gormstore.New(filepath.Join(GinkgoT().TempDir(), "concurrency.db"))
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() { Expect(s.Close()).To(Succeed()) })

	// seed writes one throttled batch the way fanOut does: the first limit
	// rows Pending and the rest Queued, all in one CreateBatch. CreatedAt is
	// stamped explicitly and one second apart so "oldest first" is a fact of
	// the fixture rather than of how fast the inserts ran.
	seed := func(batchID string, nodes int, limit int) []*store.NodeCommand {
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		cmds := make([]*store.NodeCommand, 0, nodes)
		for i := 0; i < nodes; i++ {
			phase := store.CommandPending
			if limit > 0 && i >= limit {
				phase = store.CommandQueued
			}
			cmds = append(cmds, &store.NodeCommand{
				ManagedNodeID:  fmt.Sprintf("n%d", i+1),
				Command:        store.CmdUpgrade,
				Phase:          phase,
				BatchID:        batchID,
				MaxConcurrency: limit,
				CreatedAt:      base.Add(time.Duration(i) * time.Second),
			})
		}
		Expect(s.CreateBatch(ctx, cmds)).To(Succeed())
		return cmds
	}

	phaseOf := func(id string) string {
		got, err := s.CommandGetByID(ctx, id)
		Expect(err).NotTo(HaveOccurred())
		return got.Phase
	}

	It("keeps the Queued phase CreateBatch was given", func() {
		cmds := seed("batch-a", 3, 1)
		Expect(phaseOf(cmds[0].ID)).To(Equal(store.CommandPending))
		Expect(phaseOf(cmds[1].ID)).To(Equal(store.CommandQueued))
		Expect(phaseOf(cmds[2].ID)).To(Equal(store.CommandQueued))
	})

	// GetPending is both the agent's poll and the reconnect replay. A Queued
	// row it returned would be delivered the moment the node came back, and
	// the limit would mean nothing.
	It("hides a Queued row from GetPending and from a delivery claim", func() {
		cmds := seed("batch-a", 2, 1)

		pending, err := s.GetPending(ctx, "n2")
		Expect(err).NotTo(HaveOccurred())
		Expect(pending).To(BeEmpty())

		claimed, err := s.ClaimForDelivery(ctx, cmds[1].ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeFalse())
		Expect(phaseOf(cmds[1].ID)).To(Equal(store.CommandQueued))
	})

	It("promotes nothing while the batch is at its limit", func() {
		cmds := seed("batch-a", 3, 1)

		promoted, err := s.PromoteQueuedInBatch(ctx, "batch-a", 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(promoted).To(BeEmpty())
		Expect(phaseOf(cmds[1].ID)).To(Equal(store.CommandQueued))
	})

	It("promotes the oldest queued row once a slot frees", func() {
		cmds := seed("batch-a", 3, 1)
		Expect(s.UpdateStatus(ctx, cmds[0].ID, store.CommandCompleted, "done")).To(Succeed())

		promoted, err := s.PromoteQueuedInBatch(ctx, "batch-a", 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(promoted).To(HaveLen(1))
		Expect(promoted[0].ID).To(Equal(cmds[1].ID))
		Expect(promoted[0].Phase).To(Equal(store.CommandPending))
		Expect(phaseOf(cmds[1].ID)).To(Equal(store.CommandPending))
		Expect(phaseOf(cmds[2].ID)).To(Equal(store.CommandQueued))
	})

	It("fills every free slot in one call", func() {
		cmds := seed("batch-a", 5, 2)
		Expect(s.UpdateStatus(ctx, cmds[0].ID, store.CommandCompleted, "done")).To(Succeed())
		Expect(s.UpdateStatus(ctx, cmds[1].ID, store.CommandFailed, "boom")).To(Succeed())

		promoted, err := s.PromoteQueuedInBatch(ctx, "batch-a", 2)
		Expect(err).NotTo(HaveOccurred())
		Expect(promoted).To(HaveLen(2))
		Expect(phaseOf(cmds[2].ID)).To(Equal(store.CommandPending))
		Expect(phaseOf(cmds[3].ID)).To(Equal(store.CommandPending))
		Expect(phaseOf(cmds[4].ID)).To(Equal(store.CommandQueued))
	})

	// Delivered and Running are what a node holds while it works. Counting
	// them as free would let the rollout run ahead of the nodes doing it.
	It("counts a delivered or running node as holding its slot", func() {
		cmds := seed("batch-a", 3, 2)
		Expect(s.UpdateStatus(ctx, cmds[0].ID, store.CommandDelivered, "")).To(Succeed())
		Expect(s.UpdateStatus(ctx, cmds[1].ID, store.CommandRunning, "")).To(Succeed())

		promoted, err := s.PromoteQueuedInBatch(ctx, "batch-a", 2)
		Expect(err).NotTo(HaveOccurred())
		Expect(promoted).To(BeEmpty())
	})

	It("stops when the queue is empty rather than looping", func() {
		cmds := seed("batch-a", 2, 2)
		Expect(s.UpdateStatus(ctx, cmds[0].ID, store.CommandCompleted, "done")).To(Succeed())
		Expect(s.UpdateStatus(ctx, cmds[1].ID, store.CommandCompleted, "done")).To(Succeed())

		promoted, err := s.PromoteQueuedInBatch(ctx, "batch-a", 2)
		Expect(err).NotTo(HaveOccurred())
		Expect(promoted).To(BeEmpty())
	})

	It("leaves another batch alone", func() {
		a := seed("batch-a", 2, 1)
		b := seed("batch-b", 2, 1)
		Expect(s.UpdateStatus(ctx, a[0].ID, store.CommandCompleted, "done")).To(Succeed())

		promoted, err := s.PromoteQueuedInBatch(ctx, "batch-a", 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(promoted).To(HaveLen(1))
		Expect(phaseOf(b[1].ID)).To(Equal(store.CommandQueued))
	})

	// An empty batch id is what an unbatched single-node command carries, so a
	// match on it would promote rows belonging to no rollout at all.
	It("promotes nothing for an empty batch id or a limit of zero", func() {
		cmds := seed("batch-a", 2, 1)
		Expect(s.UpdateStatus(ctx, cmds[0].ID, store.CommandCompleted, "done")).To(Succeed())

		promoted, err := s.PromoteQueuedInBatch(ctx, "", 1)
		Expect(err).NotTo(HaveOccurred())
		Expect(promoted).To(BeEmpty())

		promoted, err = s.PromoteQueuedInBatch(ctx, "batch-a", 0)
		Expect(err).NotTo(HaveOccurred())
		Expect(promoted).To(BeEmpty())
		Expect(phaseOf(cmds[1].ID)).To(Equal(store.CommandQueued))
	})

	// Without this a stopped batch would keep rows only a promotion could move,
	// and no promotion is coming: the failure that stopped it was the last
	// report anyone will make.
	It("cancels the queued rows of a batch, not only the pending ones", func() {
		cmds := seed("batch-a", 3, 1)

		n, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped: node n1 failed")
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(3))
		Expect(phaseOf(cmds[1].ID)).To(Equal(store.CommandCanceled))
		Expect(phaseOf(cmds[2].ID)).To(Equal(store.CommandCanceled))
	})

	// GetPending already refuses an overdue command, so a Queued row the sweep
	// skipped would sit in a phase nothing can advance and no delete collects.
	It("expires an overdue queued command", func() {
		past := time.Now().Add(-time.Hour)
		cmd := &store.NodeCommand{
			ManagedNodeID: "n1", Command: store.CmdUpgrade,
			Phase: store.CommandQueued, BatchID: "batch-a",
			MaxConcurrency: 1, ExpiresAt: &past,
		}
		Expect(s.CreateBatch(ctx, []*store.NodeCommand{cmd})).To(Succeed())

		Expect(s.CommandExpireBefore(ctx, "n1", time.Now())).To(Succeed())
		Expect(phaseOf(cmd.ID)).To(Equal(store.CommandExpired))
	})
})
