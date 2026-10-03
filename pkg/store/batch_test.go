package store_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// cancelSpy records what CancelBatchAfterFailure asked the store to do, so the
// rule can be checked without a database behind it.
type cancelSpy struct {
	store.CommandStore
	calls  []string
	reason string
	n      int
	err    error
}

func (c *cancelSpy) CancelPendingInBatch(_ context.Context, batchID string, reason string) (int, error) {
	c.calls = append(c.calls, batchID)
	c.reason = reason
	return c.n, c.err
}

var _ = Describe("SummarizeBatch", func() {
	cmd := func(phase string) *store.NodeCommand {
		return &store.NodeCommand{Command: store.CmdUpgrade, Phase: phase, FailFast: true}
	}

	It("keeps a batch Pending while any node still has work ahead of it", func() {
		out := store.SummarizeBatch("b", []*store.NodeCommand{
			cmd(store.CommandCompleted), cmd(store.CommandFailed), cmd(store.CommandPending),
		})
		Expect(out.Phase).To(Equal(store.CommandPending))
		Expect(out.Total).To(Equal(3))
		Expect(out.Completed).To(Equal(1))
		Expect(out.Failed).To(Equal(1))
		Expect(out.Pending).To(Equal(1))
	})

	// Delivered and Running are both "the node has it and has not answered".
	// Counting either as finished would report a rollout as over while a node
	// is still upgrading.
	It("counts a delivered command as still running, not as finished", func() {
		out := store.SummarizeBatch("b", []*store.NodeCommand{
			cmd(store.CommandCompleted), cmd(store.CommandDelivered),
		})
		Expect(out.Phase).To(Equal(store.CommandPending))
		Expect(out.Running).To(Equal(1))
	})

	It("reports Failed once every node is terminal and one failed", func() {
		out := store.SummarizeBatch("b", []*store.NodeCommand{
			cmd(store.CommandCompleted), cmd(store.CommandFailed), cmd(store.CommandCanceled),
		})
		Expect(out.Phase).To(Equal(store.CommandFailed))
		Expect(out.Canceled).To(Equal(1))
	})

	It("reports Canceled when the batch was stopped but nothing actually failed", func() {
		out := store.SummarizeBatch("b", []*store.NodeCommand{
			cmd(store.CommandCompleted), cmd(store.CommandCanceled),
		})
		Expect(out.Phase).To(Equal(store.CommandCanceled))
	})

	It("reports an expired node as a batch that did not complete", func() {
		out := store.SummarizeBatch("b", []*store.NodeCommand{
			cmd(store.CommandCompleted), cmd(store.CommandExpired),
		})
		Expect(out.Phase).To(Equal(store.CommandCanceled))
		Expect(out.Expired).To(Equal(1))
	})

	It("reports Completed only when every node succeeded", func() {
		out := store.SummarizeBatch("b", []*store.NodeCommand{
			cmd(store.CommandCompleted), cmd(store.CommandCompleted),
		})
		Expect(out.Phase).To(Equal(store.CommandCompleted))
		Expect(out.Completed).To(Equal(2))
		Expect(out.Command).To(Equal(store.CmdUpgrade))
		Expect(out.FailFast).To(BeTrue())
	})

	It("carries the batch id and the commands through", func() {
		cmds := []*store.NodeCommand{cmd(store.CommandPending)}
		out := store.SummarizeBatch("batch-a", cmds)
		Expect(out.BatchID).To(Equal("batch-a"))
		Expect(out.Commands).To(Equal(cmds))
	})
})

var _ = Describe("CancelBatchAfterFailure", func() {
	var spy *cancelSpy

	BeforeEach(func() { spy = &cancelSpy{n: 2} })

	failed := func(batchID string, failFast bool) *store.NodeCommand {
		return &store.NodeCommand{
			ID: "c1", ManagedNodeID: "node-1", BatchID: batchID,
			FailFast: failFast, Phase: store.CommandFailed,
		}
	}

	It("cancels the batch of a failed fail-fast command", func() {
		n, err := store.CancelBatchAfterFailure(context.Background(), spy, failed("batch-a", true))
		Expect(err).NotTo(HaveOccurred())
		Expect(n).To(Equal(2))
		Expect(spy.calls).To(Equal([]string{"batch-a"}))
		Expect(spy.reason).To(ContainSubstring("node-1"))
		Expect(spy.reason).To(ContainSubstring("batch-a"))
	})

	It("does nothing when the batch did not ask for fail-fast", func() {
		_, err := store.CancelBatchAfterFailure(context.Background(), spy, failed("batch-a", false))
		Expect(err).NotTo(HaveOccurred())
		Expect(spy.calls).To(BeEmpty())
	})

	It("does nothing for a command outside any batch", func() {
		_, err := store.CancelBatchAfterFailure(context.Background(), spy, failed("", true))
		Expect(err).NotTo(HaveOccurred())
		Expect(spy.calls).To(BeEmpty())
	})

	It("does nothing for a phase that is not a failure", func() {
		cmd := failed("batch-a", true)
		cmd.Phase = store.CommandCompleted
		_, err := store.CancelBatchAfterFailure(context.Background(), spy, cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(spy.calls).To(BeEmpty())
	})

	It("does nothing for a nil command or a nil store", func() {
		_, err := store.CancelBatchAfterFailure(context.Background(), spy, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(spy.calls).To(BeEmpty())

		_, err = store.CancelBatchAfterFailure(context.Background(), nil, failed("batch-a", true))
		Expect(err).NotTo(HaveOccurred())
	})

	It("surfaces a store failure to the caller", func() {
		spy.err = errors.New("database is gone")
		_, err := store.CancelBatchAfterFailure(context.Background(), spy, failed("batch-a", true))
		Expect(err).To(MatchError(ContainSubstring("database is gone")))
	})
})

var _ = Describe("IsTerminalCommandPhase", func() {
	It("names the phases a command can no longer leave", func() {
		for _, phase := range []string{
			store.CommandCompleted, store.CommandFailed,
			store.CommandExpired, store.CommandCanceled,
		} {
			Expect(store.IsTerminalCommandPhase(phase)).To(BeTrue(), phase)
		}
		for _, phase := range []string{
			store.CommandPending, store.CommandDelivered, store.CommandRunning, "",
		} {
			Expect(store.IsTerminalCommandPhase(phase)).To(BeFalse(), phase)
		}
	})
})
