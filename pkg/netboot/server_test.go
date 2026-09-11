package netboot

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("serveUntilDone", Label("netboot"), func() {
	It("stops a blocked server when the context is done", func() {
		// The real Serve blocks on a channel receive until a fatal error or
		// Shutdown, so a serve that never returns on its own is the case that
		// matters: without a shutdown on cancellation the caller hangs forever.
		release := make(chan error)
		var shutdowns atomic.Int32
		serve := func() error { return <-release }
		shutdown := func() {
			shutdowns.Add(1)
			close(release)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		done := make(chan error, 1)
		go func() { done <- serveUntilDone(ctx, serve, shutdown) }()

		var err error
		Eventually(done, 5*time.Second).Should(Receive(&err))
		Expect(err).To(MatchError(context.DeadlineExceeded))
		Expect(shutdowns.Load()).To(Equal(int32(1)))
	})

	It("returns the server's own error when it fails first", func() {
		boom := errors.New("listen udp :69: permission denied")
		var shutdowns atomic.Int32

		err := serveUntilDone(context.Background(), func() error { return boom },
			func() { shutdowns.Add(1) })

		Expect(err).To(MatchError(boom))
		Expect(shutdowns.Load()).To(BeZero())
	})
})
