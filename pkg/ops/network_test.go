package ops

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("download", Label("network"), func() {
	var origDelay time.Duration

	BeforeEach(func() {
		origDelay = downloadRetryBaseDelay
		downloadRetryBaseDelay = 200 * time.Millisecond
	})

	AfterEach(func() {
		downloadRetryBaseDelay = origDelay
	})

	// grab issues a HEAD probe before every GET; a HEAD response other than
	// 200 is not an error to grab (it just proceeds to the GET), so these
	// tests fail/succeed only the GET to control the download-attempt count.
	notFoundOnHead := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return true
		}
		return false
	}

	It("retries a transient failure and succeeds on a later attempt", func() {
		var gets atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if notFoundOnHead(w, r) {
				return
			}
			if gets.Add(1) == 1 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			w.Write([]byte("payload"))
		}))
		defer srv.Close()

		// download() is always given a destination *file* path, not a
		// directory (its only caller passes deployer.defaultIsoFile()'s full
		// ".../kairos.iso" path).
		dst := filepath.Join(GinkgoT().TempDir(), "testfile.bin")
		_, err := download(context.Background(), srv.URL+"/testfile.bin", dst)
		Expect(err).NotTo(HaveOccurred())
		Expect(gets.Load()).To(Equal(int32(2)))

		content, readErr := os.ReadFile(dst)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("payload"))
	})

	It("stops retrying and reports ctx's error when canceled after an attempt", func() {
		var gets atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Cancel through the hook rather than off a wall-clock sleep: this
		// pins the case where the attempt already failed with a transient
		// error of its own, which is the one that used to be reported
		// instead of the cancellation. A sleep raced the in-flight case and
		// made this spec flaky on a loaded runner.
		afterDownloadAttempt = cancel
		DeferCleanup(func() { afterDownloadAttempt = nil })

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if notFoundOnHead(w, r) {
				return
			}
			gets.Add(1)
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()

		dst := filepath.Join(GinkgoT().TempDir(), "testfile.bin")
		_, err := download(ctx, srv.URL+"/testfile.bin", dst)
		Expect(errors.Is(err, context.Canceled)).To(BeTrue(),
			"a caller checking for cancellation must see it, got %v", err)
		Expect(err.Error()).To(ContainSubstring("502"),
			"the attempt's own error must survive in the message, got %v", err)
		Expect(gets.Load()).To(Equal(int32(1)))
	})

	It("reports ctx's error when canceled while the transfer is in flight", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// The handler blocks until the test has canceled ctx, so the
		// cancellation is guaranteed to land while downloadOnce is still
		// waiting on the transfer rather than between two attempts.
		canceled := make(chan struct{})
		inFlight := make(chan struct{})
		var once sync.Once
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if notFoundOnHead(w, r) {
				return
			}
			once.Do(func() { close(inFlight) })
			<-canceled
			w.Write([]byte("payload"))
		}))
		defer srv.Close()

		go func() {
			defer GinkgoRecover()
			<-inFlight
			cancel()
			close(canceled)
		}()

		dst := filepath.Join(GinkgoT().TempDir(), "testfile.bin")
		_, err := download(ctx, srv.URL+"/testfile.bin", dst)
		Expect(errors.Is(err, context.Canceled)).To(BeTrue(),
			"a caller checking for cancellation must see it, got %v", err)
	})

	// kairos#5775: grab keeps a local file that is already the remote size and
	// resumes onto a shorter one, so an ISO left in the state dir by an
	// earlier run was kept or spliced with the new download.
	DescribeTable("replaces a file already at the destination",
		func(existing []byte) {
			payload := []byte(strings.Repeat("N", 4096))
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.ServeContent(w, r, "kairos.iso", time.Time{}, bytes.NewReader(payload))
			}))
			defer srv.Close()

			dst := filepath.Join(GinkgoT().TempDir(), "kairos.iso")
			Expect(os.WriteFile(dst, existing, 0o644)).To(Succeed())

			Expect(DownloadArtifact(srv.URL+"/kairos.iso", func() string { return dst })(context.Background())).To(Succeed())

			content, err := os.ReadFile(dst)
			Expect(err).NotTo(HaveOccurred())
			Expect(content).To(Equal(payload))
		},
		Entry("shorter than the artifact", []byte(strings.Repeat("O", 1000))),
		Entry("the same size as the artifact", []byte(strings.Repeat("O", 4096))),
		Entry("longer than the artifact", []byte(strings.Repeat("O", 8192))),
	)
})
