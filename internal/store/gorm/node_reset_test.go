package gorm_test

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

var _ = Describe("Gorm Store reset lifecycle", func() {
	var (
		s   *gormstore.Store
		ctx context.Context
	)

	register := func(machineID string) *store.ManagedNode {
		n := &store.ManagedNode{MachineID: machineID}
		Expect(s.Register(ctx, n)).To(Succeed())
		return n
	}

	BeforeEach(func() {
		var err error
		s, err = gormstore.New(filepath.Join(GinkgoT().TempDir(), "reset.db"))
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() { Expect(s.Close()).To(Succeed()) })

	It("SetResetPending marks the node pending and stamps the request time", func() {
		n := register("n1")
		Expect(s.SetResetPending(ctx, n.ID)).To(Succeed())

		got, err := s.NodeGetByID(ctx, n.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.ResetState).To(Equal(store.ResetStatePending))
		Expect(got.ResetRequestedAt).NotTo(BeNil())
		Expect(got.LastReset).To(BeNil())
	})

	It("advances pending -> in-progress without stamping LastReset", func() {
		n := register("n1")
		Expect(s.SetResetPending(ctx, n.ID)).To(Succeed())

		ok, err := s.AdvanceReset(ctx, n.ID, []string{store.ResetStatePending}, store.ResetStateInProgress, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())

		got, _ := s.NodeGetByID(ctx, n.ID)
		Expect(got.ResetState).To(Equal(store.ResetStateInProgress))
		Expect(got.LastReset).To(BeNil())
	})

	It("advances an in-flight reset -> done and stamps LastReset", func() {
		n := register("n1")
		Expect(s.SetResetPending(ctx, n.ID)).To(Succeed())

		ok, err := s.AdvanceReset(ctx, n.ID,
			[]string{store.ResetStatePending, store.ResetStateInProgress}, store.ResetStateDone, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())

		got, _ := s.NodeGetByID(ctx, n.ID)
		Expect(got.ResetState).To(Equal(store.ResetStateDone))
		Expect(got.LastReset).NotTo(BeNil())
	})

	It("advances an in-flight reset -> failed", func() {
		n := register("n1")
		Expect(s.SetResetPending(ctx, n.ID)).To(Succeed())

		ok, err := s.AdvanceReset(ctx, n.ID,
			[]string{store.ResetStatePending, store.ResetStateInProgress}, store.ResetStateFailed, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeTrue())

		got, _ := s.NodeGetByID(ctx, n.ID)
		Expect(got.ResetState).To(Equal(store.ResetStateFailed))
		Expect(got.LastReset).To(BeNil())
	})

	It("does not fail a reset whose request timestamp was refreshed", func() {
		n := register("n1")
		Expect(s.SetResetPending(ctx, n.ID)).To(Succeed())
		first, err := s.NodeGetByID(ctx, n.ID)
		Expect(err).NotTo(HaveOccurred())
		oldDeadline := *first.ResetRequestedAt

		time.Sleep(time.Millisecond)
		Expect(s.SetResetPending(ctx, n.ID)).To(Succeed())
		matched, err := s.FailResetBefore(ctx, n.ID, oldDeadline)
		Expect(err).NotTo(HaveOccurred())
		Expect(matched).To(BeFalse())

		got, _ := s.NodeGetByID(ctx, n.ID)
		Expect(got.ResetState).To(Equal(store.ResetStatePending))
		Expect(got.ResetRequestedAt.After(oldDeadline)).To(BeTrue())
	})

	It("fails an in-flight reset requested before the deadline", func() {
		n := register("n1")
		Expect(s.SetResetPending(ctx, n.ID)).To(Succeed())

		matched, err := s.FailResetBefore(ctx, n.ID, time.Now().Add(time.Minute))
		Expect(err).NotTo(HaveOccurred())
		Expect(matched).To(BeTrue())

		got, _ := s.NodeGetByID(ctx, n.ID)
		Expect(got.ResetState).To(Equal(store.ResetStateFailed))
	})

	It("does not transition when the current state is not in fromStates", func() {
		n := register("n1") // ResetState == "" (no reset requested)
		ok, err := s.AdvanceReset(ctx, n.ID,
			[]string{store.ResetStatePending, store.ResetStateInProgress}, store.ResetStateDone, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())

		got, _ := s.NodeGetByID(ctx, n.ID)
		Expect(got.ResetState).To(Equal(""))
		Expect(got.LastReset).To(BeNil())
	})

	It("resolves exactly once under concurrent AdvanceReset", func() {
		n := register("n1")
		Expect(s.SetResetPending(ctx, n.ID)).To(Succeed())

		const workers = 12
		wins := make(chan bool, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				ok, err := s.AdvanceReset(ctx, n.ID,
					[]string{store.ResetStatePending, store.ResetStateInProgress}, store.ResetStateDone, true)
				Expect(err).NotTo(HaveOccurred())
				wins <- ok
			}()
		}
		wg.Wait()
		close(wins)

		won := 0
		for w := range wins {
			if w {
				won++
			}
		}
		Expect(won).To(Equal(1), "exactly one concurrent resolver must perform the transition")
	})
})
