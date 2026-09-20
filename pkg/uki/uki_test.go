package uki

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kairos-io/AuroraBoot/pkg/constants"

	"github.com/kairos-io/kairos/v4/sdk/types/logger"
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

var _ = Describe("parseSelinuxOptions", func() {
	var log *logger.KairosLogger

	BeforeEach(func() {
		l := logger.NewKairosLogger("uki-test", "warn", false)
		log = &l
	})

	It("returns disabled for an empty cloud config", func() {
		enabled, mode, err := parseSelinuxOptions(log, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeFalse())
		Expect(mode).To(BeEmpty())
	})

	It("returns disabled when install.selinux is absent", func() {
		cc := "#cloud-config\nusers:\n  - name: kairos\n"
		enabled, mode, err := parseSelinuxOptions(log, cc)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeFalse())
		Expect(mode).To(BeEmpty())
	})

	It("returns disabled when install.selinux.enabled is false", func() {
		cc := "install:\n  selinux:\n    enabled: false\n    mode: enforcing\n"
		enabled, mode, err := parseSelinuxOptions(log, cc)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeFalse())
		Expect(mode).To(BeEmpty())
	})

	It("returns enabled with the given mode", func() {
		cc := "install:\n  selinux:\n    enabled: true\n    mode: enforcing\n"
		enabled, mode, err := parseSelinuxOptions(log, cc)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())
		Expect(mode).To(Equal("enforcing"))
	})

	It("defaults to permissive when mode is missing", func() {
		cc := "install:\n  selinux:\n    enabled: true\n"
		enabled, mode, err := parseSelinuxOptions(log, cc)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())
		Expect(mode).To(Equal("permissive"))
	})

	It("falls back to permissive for an invalid mode", func() {
		cc := "install:\n  selinux:\n    enabled: true\n    mode: bogus\n"
		enabled, mode, err := parseSelinuxOptions(log, cc)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())
		Expect(mode).To(Equal("permissive"))
	})

	It("picks up install.selinux from a later document in a multi-doc config", func() {
		cc := "#cloud-config\nusers:\n  - name: kairos\n---\ninstall:\n  selinux:\n    enabled: true\n    mode: enforcing\n"
		enabled, mode, err := parseSelinuxOptions(log, cc)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue())
		Expect(mode).To(Equal("enforcing"))
	})

	It("returns an error for malformed YAML", func() {
		_, _, err := parseSelinuxOptions(log, "install: [unclosed\n  bad: yaml:\n")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("UKI cmdlines with SELinux base", func() {
	var selinuxBase string

	BeforeEach(func() {
		selinuxBase = strings.Replace(constants.UkiCmdline, " selinux=0", " security=selinux selinux=1 enforcing=0 rd.cos.selinux=permissive", 1)
		Expect(selinuxBase).NotTo(Equal(constants.UkiCmdline))
	})

	It("puts the enabled fragment in every entry and never selinux=0", func() {
		entries := GetUkiCmdline(selinuxBase, "role=agent", "Kairos", []string{}, false)
		// extend mode → single entry
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Cmdline).To(ContainSubstring("security=selinux selinux=1 enforcing=0 rd.cos.selinux=permissive"))
		Expect(entries[0].Cmdline).To(ContainSubstring("role=agent"))
		Expect(entries[0].Cmdline).ToNot(ContainSubstring("selinux=0"))
	})

	It("adds one entry per extra cmdline with the patched base", func() {
		entries := GetUkiCmdline(selinuxBase, "", "Kairos", []string{"role=agent", "role=backup"}, false)
		Expect(entries).To(HaveLen(3))
		for _, e := range entries {
			Expect(e.Cmdline).To(ContainSubstring("rd.cos.selinux=permissive"))
			Expect(e.Cmdline).ToNot(ContainSubstring("selinux=0"))
		}
	})

	It("keeps unpatched base byte-identical to today (regression)", func() {
		entries := GetUkiCmdline(constants.UkiCmdline, "", "Kairos", []string{}, false)
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Cmdline).To(Equal(constants.UkiCmdline + " " + constants.UkiCmdlineInstall))
	})

	It("single-efi entries carry the patched base", func() {
		l := logger.NewKairosLogger("uki-test", "warn", false)
		entries := GetUkiSingleCmdlines(selinuxBase, "Kairos", []string{"My Entry: quiet"}, l)
		Expect(entries).To(HaveLen(1))
		Expect(entries[0].Title).To(Equal("Kairos (My Entry)"))
		Expect(entries[0].Cmdline).To(ContainSubstring("rd.cos.selinux=permissive"))
		Expect(entries[0].Cmdline).To(ContainSubstring("quiet"))
		Expect(entries[0].Cmdline).ToNot(ContainSubstring("selinux=0"))
	})

	It("EFI names stay short when the base is patched", func() {
		name := NameFromCmdline(selinuxBase, constants.ArtifactBaseName, selinuxBase+" "+constants.UkiCmdlineInstall+" quiet")
		Expect(name).ToNot(ContainSubstring("selinux"))
		Expect(name).To(HavePrefix("norole_"))
		Expect(NameFromCmdline(selinuxBase, constants.ArtifactBaseName, selinuxBase+" "+constants.UkiCmdlineInstall)).To(Equal(constants.ArtifactBaseName))
	})
})

var _ = Describe("isSelinuxSupported", func() {
	var rootfs string

	BeforeEach(func() {
		var err error
		rootfs, err = os.MkdirTemp("", "isSelinuxSupported-test-")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.MkdirAll(filepath.Join(rootfs, "etc"), 0o755)).To(Succeed())
	})

	AfterEach(func() {
		os.RemoveAll(rootfs)
	})

	DescribeTable("reports support by flavor, not by family",
		func(kairosRelease, osRelease string, want bool) {
			if kairosRelease != "" {
				Expect(os.WriteFile(filepath.Join(rootfs, "etc/kairos-release"), []byte(kairosRelease), 0o644)).To(Succeed())
			}
			if osRelease != "" {
				Expect(os.WriteFile(filepath.Join(rootfs, "etc/os-release"), []byte(osRelease), 0o644)).To(Succeed())
			}
			Expect(isSelinuxSupported(rootfs)).To(Equal(want))
		},
		Entry("fedora kairos-release",
			`KAIROS_FAMILY="redhat"
KAIROS_FLAVOR="fedora"
`, "", true),
		Entry("capitalized fedora kairos-release",
			`KAIROS_FLAVOR="Fedora"
`, "", true),
		Entry("ubuntu kairos-release (ships AppArmor, not SELinux)",
			`KAIROS_FAMILY="debian"
KAIROS_FLAVOR="ubuntu"
`, "", false),
		Entry("rocky kairos-release (redhat family, not fedora flavor)",
			`KAIROS_FAMILY="redhat"
KAIROS_FLAVOR="rockylinux"
`, "", false),
		Entry("hadron kairos-release",
			`KAIROS_FAMILY="hadron"
KAIROS_FLAVOR="hadron"
`, "", false),
		Entry("fedora via rootfs os-release when kairos-release lacks a flavor",
			`KAIROS_FAMILY="redhat"
`, `KAIROS_FLAVOR="fedora"
`, true),
		Entry("no flavor anywhere",
			`KAIROS_FAMILY="redhat"
`, `ID=rocky
`, false),
	)
})
