package ops

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrregistry "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/kairos-sdk/types/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// testImage builds a single-layer image containing one regular file owned by
// the current user, so it can be extracted without root (random.Image uses
// UID/GID 0, which makes the lchown during extraction fail when running
// rootless).
func testImage() (v1.Image, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := []byte("hello insecure registry")
	if err := tw.WriteHeader(&tar.Header{
		Name:     "hello.txt",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     int64(len(content)),
		Uid:      os.Getuid(),
		Gid:      os.Getgid(),
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(content); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}

	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
	})
	if err != nil {
		return nil, err
	}
	return mutate.AppendLayers(empty.Image, layer)
}

var _ = Describe("DumpSource against an insecure registry", Label("ops"), func() {
	var (
		server   *httptest.Server
		imageRef string
		destDir  string
	)

	BeforeEach(func() {
		internal.Log = logger.NewKairosLogger("test", "info", false)

		// A registry served over HTTPS with a self-signed certificate, like a
		// company-internal registry without a publicly trusted cert.
		server = httptest.NewTLSServer(ggcrregistry.New())

		u, err := url.Parse(server.URL)
		Expect(err).ToNot(HaveOccurred())
		imageRef = u.Host + "/test/img:latest"

		// Seed it with a small, extractable image using an insecure transport.
		img, err := testImage()
		Expect(err).ToNot(HaveOccurred())
		ref, err := name.ParseReference(imageRef, name.Insecure)
		Expect(err).ToNot(HaveOccurred())
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		Expect(remote.Write(ref, img, remote.WithTransport(tr))).To(Succeed())

		destDir, err = os.MkdirTemp("", "auroraboot-dumpsource-*")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		server.Close()
		Expect(os.RemoveAll(destDir)).To(Succeed())
	})

	It("fails without the insecure flag", func() {
		err := DumpSource("docker://"+imageRef, func() string { return destDir }, "", false)(context.Background())
		Expect(err).To(HaveOccurred())
		// It must fail because of TLS verification, not because the ref is
		// unparseable or the image is missing.
		Expect(strings.ToLower(err.Error())).To(Or(
			ContainSubstring("certificate"),
			ContainSubstring("tls"),
			ContainSubstring("x509"),
		))
	})

	It("succeeds with the insecure flag and extracts the image", func() {
		err := DumpSource("docker://"+imageRef, func() string { return destDir }, "", true)(context.Background())
		Expect(err).ToNot(HaveOccurred())

		entries, err := os.ReadDir(destDir)
		Expect(err).ToNot(HaveOccurred())
		Expect(entries).ToNot(BeEmpty())
	})
})
