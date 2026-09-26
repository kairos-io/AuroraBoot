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

	queue := func(id, nodeID, batchID, phase string) *store.NodeCommand {
		cmd := &store.NodeCommand{
			ID:            id,
			ManagedNodeID: nodeID,
			Command:       store.CmdUpgrade,
			Phase:         phase,
			BatchID:       batchID,
			FailFast:      batchID != "",
		}
		Expect(s.CommandCreate(ctx, cmd)).To(Succeed())
		return cmd
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
		queue("c1", "n1", "batch-a", store.CommandPending)

		got, err := s.CommandGetByID(ctx, "c1")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.BatchID).To(Equal("batch-a"))
		Expect(got.FailFast).To(BeTrue())
	})

	It("ListByBatch returns only the commands of that batch", func() {
		queue("c1", "n1", "batch-a", store.CommandPending)
		queue("c2", "n2", "batch-a", store.CommandPending)
		queue("c3", "n3", "batch-b", store.CommandPending)
		queue("c4", "n4", "", store.CommandPending)

		got, err := s.ListByBatch(ctx, "batch-a")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(HaveLen(2))
		ids := []string{got[0].ID, got[1].ID}
		Expect(ids).To(ConsistOf("c1", "c2"))
	})

	// An empty batch id is what an unbatched, single-node command carries. A
	// query on it must not match every such command, or one failed single-node
	// upgrade would cancel every other queued single-node command.
	It("ListByBatch on an empty id matches nothing", func() {
		queue("c1", "n1", "", store.CommandPending)

		got, err := s.ListByBatch(ctx, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(BeEmpty())
	})

	It("CancelPendingInBatch moves only the Pending rows of the batch", func() {
		queue("pending-1", "n1", "batch-a", store.CommandPending)
		queue("pending-2", "n2", "batch-a", store.CommandPending)
		queue("delivered", "n3", "batch-a", store.CommandDelivered)
		queue("running", "n4", "batch-a", store.CommandRunning)
		queue("failed", "n5", "batch-a", store.CommandFailed)
		queue("other-batch", "n6", "batch-b", store.CommandPending)
		queue("unbatched", "n7", "", store.CommandPending)

		n, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped: node n5 failed")
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(2))

		Expect(phaseOf("pending-1")).To(Equal(store.CommandCanceled))
		Expect(phaseOf("pending-2")).To(Equal(store.CommandCanceled))
		Expect(phaseOf("delivered")).To(Equal(store.CommandDelivered))
		Expect(phaseOf("running")).To(Equal(store.CommandRunning))
		Expect(phaseOf("failed")).To(Equal(store.CommandFailed))
		Expect(phaseOf("other-batch")).To(Equal(store.CommandPending))
		Expect(phaseOf("unbatched")).To(Equal(store.CommandPending))
	})

	It("CancelPendingInBatch records the reason and stamps the completion time", func() {
		queue("c1", "n1", "batch-a", store.CommandPending)

		_, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped: node n9 failed")
		Expect(err).NotTo(HaveOccurred())

		got, err := s.CommandGetByID(ctx, "c1")
		Expect(err).NotTo(HaveOccurred())
		Expect(got.Result).To(Equal("stopped: node n9 failed"))
		Expect(got.CompletedAt).NotTo(BeNil())
	})

	It("CancelPendingInBatch on an empty batch id cancels nothing", func() {
		queue("c1", "n1", "", store.CommandPending)

		n, err := s.CancelPendingInBatch(ctx, "", "should not happen")
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(BeZero())
		Expect(phaseOf("c1")).To(Equal(store.CommandPending))
	})

	// The cancel and ClaimForDelivery are the same compare-and-set on the same
	// row: whichever lands first leaves no Pending row for the other, so a
	// command is never both canceled and pushed to a node.
	It("a cancel and a delivery claim cannot both win the same command", func() {
		queue("c1", "n1", "batch-a", store.CommandPending)

		n, err := s.CancelPendingInBatch(ctx, "batch-a", "stopped")
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(1))

		claimed, err := s.ClaimForDelivery(ctx, "c1")
		Expect(err).NotTo(HaveOccurred())
		Expect(claimed).To(BeFalse())
		Expect(phaseOf("c1")).To(Equal(store.CommandCanceled))
	})

	It("CommandDeleteTerminal clears canceled commands with the rest of the history", func() {
		queue("done", "n1", "batch-a", store.CommandCompleted)
		queue("canceled", "n1", "batch-a", store.CommandCanceled)
		queue("still-pending", "n1", "batch-a", store.CommandPending)

		Expect(s.CommandDeleteTerminal(ctx, "n1")).To(Succeed())

		left, err := s.ListByNode(ctx, "n1")
		Expect(err).NotTo(HaveOccurred())
		Expect(left).To(HaveLen(1))
		Expect(left[0].ID).To(Equal("still-pending"))
	})
})
