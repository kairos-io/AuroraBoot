package ops

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		// directory (its only caller passes deployer.getIsoFile()'s full
		// ".../kairos.iso" path).
		dst := filepath.Join(GinkgoT().TempDir(), "testfile.bin")
		_, err := download(context.Background(), srv.URL+"/testfile.bin", dst)
		Expect(err).NotTo(HaveOccurred())
		Expect(gets.Load()).To(Equal(int32(2)))

		content, readErr := os.ReadFile(dst)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(string(content)).To(Equal("payload"))
	})

	It("stops retrying and reports ctx's error when canceled during backoff", func() {
		var gets atomic.Int32
		ctx, cancel := context.WithCancel(context.Background())

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if notFoundOnHead(w, r) {
				return
			}
			gets.Add(1)
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()

		dst := filepath.Join(GinkgoT().TempDir(), "testfile.bin")

		go func() {
			defer GinkgoRecover()
			// Give the first attempt's GET time to fail and enter backoff,
			// then cancel well before the (200ms) backoff elapses.
			time.Sleep(30 * time.Millisecond)
			cancel()
		}()

		_, err := download(ctx, srv.URL+"/testfile.bin", dst)
		Expect(errors.Is(err, context.Canceled)).To(BeTrue())
		Expect(gets.Load()).To(Equal(int32(1)))
	})
})
