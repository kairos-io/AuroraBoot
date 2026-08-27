package uki

import (
	"context"
	"os"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/pkg/extensions"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("catalog extensions", func() {
	It("rejects extension requests for non-ISO output", func() {
		opts := validTestOptions()
		opts.Extensions = []extensions.Request{{Name: "tool"}}
		opts.ExtensionsCatalog = "catalog.yaml"
		Expect(opts.validate()).To(MatchError("extensions are only supported for iso artifacts"))
	})

	It("requires a catalog when extensions are requested", func() {
		opts := validTestOptions()
		opts.OutputType = "iso"
		opts.Extensions = []extensions.Request{{Name: "tool"}}
		Expect(opts.validate()).To(MatchError("extensions catalog is required when extensions are requested"))
	})

	It("materializes extensions into the staged ISO root with the target architecture", func() {
		original := materializeExtensions
		DeferCleanup(func() { materializeExtensions = original })
		stagedRoot := tTempDir()
		called := false
		materializeExtensions = func(_ context.Context, catalog string, requests []extensions.Request, arch, destination string, insecure bool) ([]string, error) {
			called = true
			Expect(catalog).To(Equal("catalog.yaml"))
			Expect(requests).To(Equal([]extensions.Request{{Name: "tool", Version: "v2"}}))
			Expect(arch).To(Equal("arm64"))
			Expect(destination).To(Equal(stagedRoot))
			Expect(insecure).To(BeTrue())
			return []string{filepath.Join(destination, "tool.sysext.raw")}, nil
		}

		Expect(stageExtensions(context.Background(), "catalog.yaml", []extensions.Request{{Name: "tool", Version: "v2"}}, "arm64", stagedRoot, true)).To(Succeed())
		Expect(called).To(BeTrue())
	})
})

func validTestOptions() Options {
	dir := tTempDir()
	for _, name := range []string{"sb.key", "sb.pem", "pcr.key"} {
		Expect(os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o600)).To(Succeed())
	}
	return Options{
		Source:           "dir:rootfs",
		SBKey:            filepath.Join(dir, "sb.key"),
		SBCert:           filepath.Join(dir, "sb.pem"),
		TPMPCRPrivateKey: filepath.Join(dir, "pcr.key"),
	}
}

func tTempDir() string {
	dir, err := os.MkdirTemp("", "uki-extension-test-")
	Expect(err).ToNot(HaveOccurred())
	DeferCleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

var _ = Describe("sumFileSizes", func() {
	var tempDir string
	var err error

	BeforeEach(func() {
		tempDir, err = os.MkdirTemp("", "sumFileSizes-test-")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	It("should account for filesystem overhead", func() {
		// Create a file that is 1 MB (1048576 bytes)
		file1 := filepath.Join(tempDir, "file1")
		err := os.WriteFile(file1, make([]byte, 1048576), 0644)
		Expect(err).ToNot(HaveOccurred())

		filesMap := map[string][]string{
			"dir1": {file1},
		}

		sizeMB, err := sumFileSizes(filesMap)
		Expect(err).ToNot(HaveOccurred())
		// Should be more than 1 MB due to filesystem overhead
		Expect(sizeMB).To(BeNumerically(">", int64(1)))
	})

	It("should handle larger files with overhead", func() {
		// Create a file that is exactly 5 MB (5242880 bytes)
		file1 := filepath.Join(tempDir, "file1")
		err := os.WriteFile(file1, make([]byte, 5*1024*1024), 0644)
		Expect(err).ToNot(HaveOccurred())

		filesMap := map[string][]string{
			"dir1": {file1},
		}

		sizeMB, err := sumFileSizes(filesMap)
		Expect(err).ToNot(HaveOccurred())
		// Should be more than 5 MB due to filesystem overhead
		Expect(sizeMB).To(BeNumerically(">", int64(5)))
	})

	It("should sum multiple files with overhead", func() {
		// Create file1: 1.5 MB
		file1 := filepath.Join(tempDir, "file1")
		err := os.WriteFile(file1, make([]byte, 1536*1024), 0644) // 1.5 MB
		Expect(err).ToNot(HaveOccurred())

		// Create file2: 2.25 MB
		file2 := filepath.Join(tempDir, "file2")
		err = os.WriteFile(file2, make([]byte, 2355200), 0644) // ~2.25 MB
		Expect(err).ToNot(HaveOccurred())

		filesMap := map[string][]string{
			"dir1": {file1},
			"dir2": {file2},
		}

		sizeMB, err := sumFileSizes(filesMap)
		Expect(err).ToNot(HaveOccurred())
		// Total: ~3.75 MB + overhead, should be at least 4 MB
		Expect(sizeMB).To(BeNumerically(">=", int64(4)))
	})

	It("should handle fractional megabytes with overhead", func() {
		// Create a file that is 50.5 MB (52953088 bytes)
		file1 := filepath.Join(tempDir, "file1")
		err := os.WriteFile(file1, make([]byte, 52953088), 0644)
		Expect(err).ToNot(HaveOccurred())

		filesMap := map[string][]string{
			"dir1": {file1},
		}

		sizeMB, err := sumFileSizes(filesMap)
		Expect(err).ToNot(HaveOccurred())
		// Should be more than 50.5 MB due to filesystem overhead
		Expect(sizeMB).To(BeNumerically(">=", int64(51)))
	})

	It("should return error for non-existent file", func() {
		filesMap := map[string][]string{
			"dir1": {"/nonexistent/file"},
		}

		_, err := sumFileSizes(filesMap)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("finding file info"))
	})
})

var _ = Describe("absolutizePaths", func() {
	It("rewrites relative key/cert/splash paths to absolute", func() {
		opts := &Options{
			TPMPCRPrivateKey: "data/keys/production/tpm2-pcr-private.pem",
			SBKey:            "data/keys/db.key",
			SBCert:           "data/keys/db.pem",
			PublicKeysDir:    "data/keys",
			Splash:           "data/splash.bmp",
		}
		Expect(absolutizePaths(opts)).To(Succeed())

		Expect(filepath.IsAbs(opts.TPMPCRPrivateKey)).To(BeTrue())
		Expect(filepath.IsAbs(opts.SBKey)).To(BeTrue())
		Expect(filepath.IsAbs(opts.SBCert)).To(BeTrue())
		Expect(filepath.IsAbs(opts.PublicKeysDir)).To(BeTrue())
		Expect(filepath.IsAbs(opts.Splash)).To(BeTrue())
		Expect(opts.TPMPCRPrivateKey).To(HaveSuffix("/data/keys/production/tpm2-pcr-private.pem"))
	})

	It("rewrites relative overlay and output paths to absolute", func() {
		opts := &Options{
			OverlayRootfs: "data/overlay-rootfs",
			OverlayISO:    "data/overlay-iso",
			OutputDir:     "build/out",
		}
		Expect(absolutizePaths(opts)).To(Succeed())

		Expect(filepath.IsAbs(opts.OverlayRootfs)).To(BeTrue())
		Expect(filepath.IsAbs(opts.OverlayISO)).To(BeTrue())
		Expect(filepath.IsAbs(opts.OutputDir)).To(BeTrue())
		Expect(opts.OutputDir).To(HaveSuffix("/build/out"))
	})

	It("leaves empty values and pkcs11 URIs untouched", func() {
		opts := &Options{
			SBKey:            "pkcs11:token=mytoken;object=mykey",
			TPMPCRPrivateKey: "",
			OverlayISO:       "",
			OutputDir:        "",
		}
		Expect(absolutizePaths(opts)).To(Succeed())
		Expect(opts.SBKey).To(Equal("pkcs11:token=mytoken;object=mykey"))
		Expect(opts.TPMPCRPrivateKey).To(BeEmpty())
		Expect(opts.OverlayISO).To(BeEmpty())
		Expect(opts.OutputDir).To(BeEmpty())
	})

	It("leaves already-absolute paths unchanged", func() {
		opts := &Options{TPMPCRPrivateKey: "/data/keys/production/tpm2-pcr-private.pem"}
		Expect(absolutizePaths(opts)).To(Succeed())
		Expect(opts.TPMPCRPrivateKey).To(Equal("/data/keys/production/tpm2-pcr-private.pem"))
	})
})
