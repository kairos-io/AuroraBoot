package ops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/pkg/schema"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("InjectISO", Label("iso"), func() {
	// InjectISO must not mutate the process working directory: builds run as
	// concurrent goroutines in `web` mode and a global os.Chdir would corrupt
	// sibling builds. This spec drives the cloud-config injection branch (a
	// non-empty config.yaml in dst) and asserts the cwd is identical before and
	// after, and that the injected config is present in the ISO.
	It("injects cloud-config without changing the process working directory", func() {
		if _, err := exec.LookPath("xorriso"); err != nil {
			Skip("xorriso not installed")
		}

		dst := GinkgoT().TempDir()

		// A non-empty config.yaml in dst triggers the injection branch.
		Expect(os.WriteFile(filepath.Join(dst, "config.yaml"),
			[]byte("#cloud-config\nhostname: injected\n"), 0o644)).To(Succeed())

		// Build a minimal source ISO from a payload dir so xorriso has a real
		// -indev/-outdev to replay against.
		payload := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(payload, "placeholder"), []byte("x"), 0o644)).To(Succeed())
		isoFile := filepath.Join(GinkgoT().TempDir(), "source.iso")
		mk := exec.Command("xorriso", "-as", "mkisofs", "-o", isoFile, payload)
		out, err := mk.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(out))

		beforeWD, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		fn := InjectISO(
			func() string { return dst },
			func() string { return isoFile },
			schema.ISO{},
		)
		Expect(fn(context.Background())).To(Succeed())

		afterWD, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(afterWD).To(Equal(beforeWD), "InjectISO must not change the process cwd")

		// The injected config.yaml must now be present inside the ISO.
		ls := exec.Command("xorriso", "-indev", isoFile, "-find", "/config.yaml")
		lsOut, lsErr := ls.CombinedOutput()
		Expect(lsErr).NotTo(HaveOccurred(), string(lsOut))
		Expect(string(lsOut)).To(ContainSubstring("config.yaml"))
	})

	It("does not change the process working directory when xorriso fails", func() {
		dst := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dst, "config.yaml"),
			[]byte("#cloud-config\nhostname: injected\n"), 0o644)).To(Succeed())

		beforeWD, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())

		// A bogus iso path makes xorriso fail; cwd must still be preserved.
		fn := InjectISO(
			func() string { return dst },
			func() string { return filepath.Join(GinkgoT().TempDir(), "does-not-exist.iso") },
			schema.ISO{},
		)
		_ = fn(context.Background())

		afterWD, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		Expect(afterWD).To(Equal(beforeWD), "InjectISO must not change the process cwd even on failure")
	})
})
