package uki

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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

var _ = Describe("absolutizeKeyPaths", func() {
	It("rewrites relative key/cert/splash paths to absolute", func() {
		opts := &Options{
			TPMPCRPrivateKey: "data/keys/production/tpm2-pcr-private.pem",
			SBKey:            "data/keys/db.key",
			SBCert:           "data/keys/db.pem",
			PublicKeysDir:    "data/keys",
			Splash:           "data/splash.bmp",
		}
		Expect(absolutizeKeyPaths(opts)).To(Succeed())

		Expect(filepath.IsAbs(opts.TPMPCRPrivateKey)).To(BeTrue())
		Expect(filepath.IsAbs(opts.SBKey)).To(BeTrue())
		Expect(filepath.IsAbs(opts.SBCert)).To(BeTrue())
		Expect(filepath.IsAbs(opts.PublicKeysDir)).To(BeTrue())
		Expect(filepath.IsAbs(opts.Splash)).To(BeTrue())
		Expect(opts.TPMPCRPrivateKey).To(HaveSuffix("/data/keys/production/tpm2-pcr-private.pem"))
	})

	It("leaves empty values and pkcs11 URIs untouched", func() {
		opts := &Options{
			SBKey:            "pkcs11:token=mytoken;object=mykey",
			TPMPCRPrivateKey: "",
		}
		Expect(absolutizeKeyPaths(opts)).To(Succeed())
		Expect(opts.SBKey).To(Equal("pkcs11:token=mytoken;object=mykey"))
		Expect(opts.TPMPCRPrivateKey).To(BeEmpty())
	})

	It("leaves already-absolute paths unchanged", func() {
		opts := &Options{TPMPCRPrivateKey: "/data/keys/production/tpm2-pcr-private.pem"}
		Expect(absolutizeKeyPaths(opts)).To(Succeed())
		Expect(opts.TPMPCRPrivateKey).To(Equal("/data/keys/production/tpm2-pcr-private.pem"))
	})
})
