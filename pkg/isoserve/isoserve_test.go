package isoserve_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/isoserve"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("isoserve.Server", func() {
	var (
		srv     *isoserve.Server
		ts      *httptest.Server
		isoPath string
		payload []byte
	)

	BeforeEach(func() {
		dir := GinkgoT().TempDir()
		isoPath = filepath.Join(dir, "kairos.iso")
		payload = []byte(strings.Repeat("AURORABOOT-ISO-BYTES-", 512))
		Expect(os.WriteFile(isoPath, payload, 0644)).To(Succeed())

		// BaseURL is set after ts is up so Register produces a fetchable URL.
		srv = isoserve.New(isoserve.Config{})
		ts = httptest.NewServer(srv.Handler())
		// Re-create with the real base URL now that we know it.
		srv = isoserve.New(isoserve.Config{BaseURL: ts.URL})
		ts.Config.Handler = srv.Handler()
	})

	AfterEach(func() {
		ts.Close()
	})

	get := func(url string, headers map[string]string) (*http.Response, []byte) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		Expect(err).NotTo(HaveOccurred())
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		Expect(err).NotTo(HaveOccurred())
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		_ = resp.Body.Close()
		return resp, body
	}

	It("serves the exact bytes for a valid token", func() {
		url, token, err := srv.Register(isoPath, time.Minute)
		Expect(err).NotTo(HaveOccurred())
		Expect(token).NotTo(BeEmpty())
		Expect(url).To(HavePrefix(ts.URL + "/redfish/iso/"))
		Expect(url).To(HaveSuffix("/kairos.iso"))

		resp, body := get(url, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(body).To(Equal(payload))
	})

	It("supports Range requests", func() {
		url, _, err := srv.Register(isoPath, time.Minute)
		Expect(err).NotTo(HaveOccurred())

		resp, body := get(url, map[string]string{"Range": "bytes=0-9"})
		Expect(resp.StatusCode).To(Equal(http.StatusPartialContent))
		Expect(body).To(Equal(payload[0:10]))
		Expect(resp.Header.Get("Content-Range")).To(Equal(fmt.Sprintf("bytes 0-9/%d", len(payload))))
	})

	It("returns 404 for a wrong/unknown token", func() {
		resp, _ := get(ts.URL+"/redfish/iso/not-a-real-token/kairos.iso", nil)
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	It("returns 404 once the token has expired", func() {
		url, _, err := srv.Register(isoPath, 20*time.Millisecond)
		Expect(err).NotTo(HaveOccurred())

		resp, body := get(url, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(body).To(Equal(payload))

		Eventually(func() int {
			resp, _ := get(url, nil)
			return resp.StatusCode
		}, time.Second, 10*time.Millisecond).Should(Equal(http.StatusNotFound))
	})

	It("returns 404 after a token is revoked", func() {
		url, token, err := srv.Register(isoPath, time.Minute)
		Expect(err).NotTo(HaveOccurred())

		resp, _ := get(url, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		srv.Revoke(token)

		resp, _ = get(url, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	It("cannot be escaped with a ../ path after the token", func() {
		// Plant a secret file alongside the ISO; a traversal attempt must not
		// reach it. The token binds exactly one file, so the path after it is
		// inert.
		secret := filepath.Join(filepath.Dir(isoPath), "secret.txt")
		Expect(os.WriteFile(secret, []byte("TOP-SECRET"), 0644)).To(Succeed())

		_, token, err := srv.Register(isoPath, time.Minute)
		Expect(err).NotTo(HaveOccurred())

		// Percent-encode the traversal so the Go client does not collapse it and
		// the server actually receives "../../secret.txt" after the token. The
		// server ignores everything after the token and serves the bound ISO; it
		// never touches the secret.
		escape := fmt.Sprintf("%s/redfish/iso/%s/%%2e%%2e/%%2e%%2e/secret.txt", ts.URL, token)
		resp, body := get(escape, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(body).To(Equal(payload))
		Expect(body).NotTo(ContainSubstring("TOP-SECRET"))
	})

	It("rejects a non-absolute or non-existent path at registration", func() {
		_, _, err := srv.Register("relative/path.iso", time.Minute)
		Expect(err).To(HaveOccurred())

		_, _, err = srv.Register(filepath.Join(filepath.Dir(isoPath), "missing.iso"), time.Minute)
		Expect(err).To(HaveOccurred())

		_, _, err = srv.Register(isoPath, 0)
		Expect(err).To(HaveOccurred())
	})

	It("is safe under concurrent reads of one token", func() {
		url, _, err := srv.Register(isoPath, time.Minute)
		Expect(err).NotTo(HaveOccurred())

		const readers = 16
		var wg sync.WaitGroup
		errs := make(chan error, readers)
		for i := 0; i < readers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				resp, body := get(url, nil)
				if resp.StatusCode != http.StatusOK {
					errs <- fmt.Errorf("status %d", resp.StatusCode)
					return
				}
				if string(body) != string(payload) {
					errs <- fmt.Errorf("body mismatch")
				}
			}()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			Expect(e).NotTo(HaveOccurred())
		}
	})

	It("starts and shuts down its own listener", func() {
		lifeSrv := isoserve.New(isoserve.Config{BindAddr: "127.0.0.1:0"})
		Expect(lifeSrv.Start(context.Background())).To(Succeed())
		Expect(lifeSrv.Addr()).NotTo(BeNil())

		// Point BaseURL at the now-known address by re-deriving the URL manually.
		url, _, err := lifeSrv.Register(isoPath, time.Minute)
		Expect(err).NotTo(HaveOccurred())
		// BaseURL was empty, so the URL is host-relative; fetch via the listener.
		fetch := "http://" + lifeSrv.Addr().String() + url
		resp, body := get(fetch, nil)
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(body).To(Equal(payload))

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		Expect(lifeSrv.Shutdown(ctx)).To(Succeed())
	})

	It("requires a bind address to start", func() {
		bad := isoserve.New(isoserve.Config{})
		Expect(bad.Start(context.Background())).To(HaveOccurred())
	})
})
