package ops

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// freeAddr asks the OS for a currently-free TCP port and returns it as a
// host:port string, so two ServeArtifacts instances can bind distinct ports.
func freeAddr() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	addr := l.Addr().String()
	Expect(l.Close()).To(Succeed())
	return addr
}

var _ = Describe("ServeArtifacts", Label("network"), func() {
	// ServeArtifacts must register its file handler on a private mux, not the
	// global http.DefaultServeMux: registering "/" globally panics on the second
	// call. Starting two instances proves no global state is shared.
	It("starts two instances without panicking and serves the configured dir", func() {
		dirA := GinkgoT().TempDir()
		dirB := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dirA, "a.txt"), []byte("from-A"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(dirB, "b.txt"), []byte("from-B"), 0o644)).To(Succeed())

		addrA := freeAddr()
		addrB := freeAddr()

		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		start := func(addr, dir string) {
			fn := ServeArtifacts(addr, func() string { return dir })
			go func() {
				defer GinkgoRecover()
				// ListenAndServe returns ErrServerClosed on Shutdown; both
				// instances registering "/" must NOT panic on a shared mux.
				_ = fn(ctx)
			}()
		}
		start(addrA, dirA)
		start(addrB, dirB)

		get := func(addr, path string) (int, string) {
			var code int
			var body string
			Eventually(func() error {
				resp, err := http.Get(fmt.Sprintf("http://%s/%s", addr, path))
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				b, _ := io.ReadAll(resp.Body)
				code = resp.StatusCode
				body = string(b)
				return nil
			}, 5*time.Second, 50*time.Millisecond).Should(Succeed())
			return code, body
		}

		codeA, bodyA := get(addrA, "a.txt")
		Expect(codeA).To(Equal(http.StatusOK))
		Expect(bodyA).To(Equal("from-A"))

		codeB, bodyB := get(addrB, "b.txt")
		Expect(codeB).To(Equal(http.StatusOK))
		Expect(bodyB).To(Equal("from-B"))
	})
})
