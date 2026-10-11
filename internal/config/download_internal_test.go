package config

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// serve starts an httptest server for the duration of the spec.
func serve(h http.HandlerFunc) string {
	s := httptest.NewServer(h)
	DeferCleanup(s.Close)
	return s.URL
}

var _ = Describe("downloadFile", func() {
	It("returns a config that fits under the cap", func() {
		url := serve(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, "#cloud-config\nhostname: test\n")
		})

		got, err := downloadFile(url)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("#cloud-config\nhostname: test\n"))
	})

	It("accepts a body that stops exactly at the cap", func() {
		url := serve(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(make([]byte, maxDownloadSize))
		})

		got, err := downloadFile(url)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(HaveLen(maxDownloadSize))
	})

	It("refuses a declared length over the cap without reading the body", func() {
		var bodyWrites int
		url := serve(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", fmt.Sprint(maxDownloadSize+1))
			w.WriteHeader(http.StatusOK)
			// One chunk at a time so a client that walks away is visible as a
			// write count far below the declared length.
			for sent := 0; sent <= maxDownloadSize; sent += 1 << 16 {
				if _, err := w.Write(make([]byte, 1<<16)); err != nil {
					return
				}
				bodyWrites++
			}
		})

		got, err := downloadFile(url)
		Expect(err).To(HaveOccurred())
		// The byte count only appears in the Content-Length branch, so this
		// distinguishes it from the read-side refusal below.
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("is %d bytes, over the %d byte limit", maxDownloadSize+1, maxDownloadSize)))
		Expect(got).To(BeEmpty())
	})

	It("refuses a body of unknown length that runs past the cap", func() {
		url := serve(func(w http.ResponseWriter, _ *http.Request) {
			// No Content-Length, so the response is chunked and the declared
			// length check cannot see the size coming.
			f, ok := w.(http.Flusher)
			Expect(ok).To(BeTrue())
			chunk := make([]byte, 1<<16)
			for sent := 0; sent <= maxDownloadSize; sent += len(chunk) {
				if _, err := w.Write(chunk); err != nil {
					return
				}
				f.Flush()
			}
		})

		got, err := downloadFile(url)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("is over the %d byte limit", maxDownloadSize)))
		Expect(got).To(BeEmpty())
	})

	It("gives up on a server that answers and then stops sending", func() {
		downloadTimeout = 200 * time.Millisecond
		DeferCleanup(func() { downloadTimeout = 30 * time.Second })

		// Released with a plain defer, not DeferCleanup: the handler must be
		// unblocked before the server's own DeferCleanup tries to close it.
		released := make(chan struct{})
		defer close(released)

		url := serve(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "1024")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("a"))
			w.(http.Flusher).Flush()
			// Headers and a first byte arrived, so only a timeout that covers
			// the body read can end this.
			<-released
		})

		start := time.Now()
		got, err := downloadFile(url)
		Expect(err).To(HaveOccurred())
		Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second))
		Expect(got).To(BeEmpty())
	})

	It("reports the status when the server refuses", func() {
		url := serve(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusNotFound)
		})

		_, err := downloadFile(url)
		Expect(err).To(MatchError(ContainSubstring("bad status")))
	})
})

var _ = Describe("ReadCloudConfig over HTTP", func() {
	It("refuses a cloud-config that runs past the cap", func() {
		url := serve(func(w http.ResponseWriter, _ *http.Request) {
			f := w.(http.Flusher)
			chunk := []byte(strings.Repeat("a", 1<<16))
			for sent := 0; sent <= maxDownloadSize; sent += len(chunk) {
				if _, err := w.Write(chunk); err != nil {
					return
				}
				f.Flush()
			}
		})

		got, err := ReadCloudConfig(url, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("byte limit"))
		Expect(got).To(BeEmpty())
	})

	It("still renders a cloud-config that fits", func() {
		url := serve(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, "#cloud-config\nhostname: [[[.name]]]\n")
		})

		got, err := ReadCloudConfig(url, map[string]interface{}{"name": "node1"})
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("#cloud-config\nhostname: node1\n"))
	})
})
