package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
)

// buildRaw runs an `auroraboot sysext|confext` build, retrying on the host
// loop-device contention flake. systemd-repart --make-ddi allocates host loop
// devices for the final "Applying changes" step; on a host with few loop
// devices, back-to-back builds race ("Failed to make loopback device: Device
// or resource busy"). That's a host-resource flake, not a logic bug — the same
// retry guards the REST tier (see buildExtension / isLoopbackRace). A genuine
// failure (e.g. bad arch) isn't a loopback race, so it returns on the first try.
func buildRaw(aurora *Auroraboot, args ...string) (string, error) {
	var out string
	var err error
	for attempt := 1; attempt <= 4; attempt++ {
		out, err = aurora.ContainerRun("auroraboot", args...)
		if err == nil || !isLoopbackRace(out) {
			return out, err
		}
		// Give the host loop subsystem a moment to release devices.
		time.Sleep(time.Duration(attempt*3) * time.Second)
	}
	return out, err
}

// These specs exercise the `auroraboot sysext` / `auroraboot confext`
// subcommands that build a signed .raw system/config extension from the last
// layer of a container image. They are the CLI half of the extensions
// feature — the REST + phonehome half lives in extensions_web_test.go.
//
// The container image used as the extraction source is intentionally tiny
// (alpine) so the specs stay fast: each build is a pull + systemd-repart run.
var _ = Describe("sysext/confext generation", Label("sysext", "e2e"), func() {
	const srcImage = "alpine:3.21"

	var resultDir string
	var err error
	var aurora *Auroraboot

	BeforeEach(func() {
		format.MaxLength = 0
		resultDir, err = os.MkdirTemp("", "auroraboot-sysext-test-")
		Expect(err).ToNot(HaveOccurred())
		aurora = NewAuroraboot(resultDir)
	})

	AfterEach(func() {
		os.RemoveAll(resultDir)
	})

	// Regression guard for the flag-ordering bug: urfave/cli v2 only parses
	// flags that precede the positional args, so `--output <dir>` placed after
	// `<name> <container>` was silently dropped and the .raw landed in the
	// process cwd instead of the requested output dir.
	It("builds a sysext into the --output dir (flags before positional args)", func() {
		out, err := buildRaw(aurora, "sysext",
			"--arch", "amd64",
			"--output", resultDir,
			"e2e-tools", srcImage,
		)
		Expect(err).ToNot(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("Done sysext creation"), out)

		raw := filepath.Join(resultDir, "e2e-tools.sysext.raw")
		info, statErr := os.Stat(raw)
		Expect(statErr).ToNot(HaveOccurred(), "expected the .raw in the --output dir, not the container cwd")
		Expect(info.Size()).To(BeNumerically(">", 0))
	})

	It("builds a confext into the --output dir", func() {
		out, err := buildRaw(aurora, "confext",
			"--arch", "amd64",
			"--output", resultDir,
			"e2e-confext", srcImage,
		)
		Expect(err).ToNot(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("Done confext creation"), out)

		_, statErr := os.Stat(filepath.Join(resultDir, "e2e-confext.confext.raw"))
		Expect(statErr).ToNot(HaveOccurred())
	})

	It("extends the sysext allowlist with --include-path", func() {
		out, err := buildRaw(aurora, "sysext",
			"--debug",
			"--arch", "amd64",
			"--output", resultDir,
			"--include-path", "/opt",
			"--include-path", "/srv",
			"e2e-paths", srcImage,
		)
		Expect(err).ToNot(HaveOccurred(), out)
		// The handler logs the extended allowlist at debug level.
		Expect(out).To(ContainSubstring("extending sysext allowlist"), out)
		_, statErr := os.Stat(filepath.Join(resultDir, "e2e-paths.sysext.raw"))
		Expect(statErr).ToNot(HaveOccurred())
	})

	It("keeps --with-opt working as a deprecated alias for --include-path=/opt", func() {
		out, err := buildRaw(aurora, "sysext",
			"--debug",
			"--arch", "amd64",
			"--output", resultDir,
			"--with-opt",
			"e2e-withopt", srcImage,
		)
		Expect(err).ToNot(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("--with-opt is deprecated"), out)
		_, statErr := os.Stat(filepath.Join(resultDir, "e2e-withopt.sysext.raw"))
		Expect(statErr).ToNot(HaveOccurred())
	})

	It("signs the sysext when a private key and certificate are given", func() {
		// Generate a signing key/cert pair (produces db.key + db.pem in resultDir).
		out, err := aurora.ContainerRun("auroraboot", "genkey", "-o", resultDir, "e2e-signing")
		Expect(err).ToNot(HaveOccurred(), out)
		Expect(filepath.Join(resultDir, "db.key")).To(BeAnExistingFile())
		Expect(filepath.Join(resultDir, "db.pem")).To(BeAnExistingFile())

		// Build an unsigned sysext for a size baseline.
		out, err = buildRaw(aurora, "sysext",
			"--arch", "amd64", "--output", resultDir, "e2e-unsigned", srcImage)
		Expect(err).ToNot(HaveOccurred(), out)
		unsigned, statErr := os.Stat(filepath.Join(resultDir, "e2e-unsigned.sysext.raw"))
		Expect(statErr).ToNot(HaveOccurred())

		// Build a signed sysext from the same source.
		out, err = buildRaw(aurora, "sysext",
			"--arch", "amd64",
			"--output", resultDir,
			"--private-key", filepath.Join(resultDir, "db.key"),
			"--certificate", filepath.Join(resultDir, "db.pem"),
			"e2e-signed", srcImage,
		)
		Expect(err).ToNot(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("Done sysext creation"), out)

		signed, statErr := os.Stat(filepath.Join(resultDir, "e2e-signed.sysext.raw"))
		Expect(statErr).ToNot(HaveOccurred())
		// The signature partition makes the signed image strictly larger.
		Expect(signed.Size()).To(BeNumerically(">", unsigned.Size()),
			fmt.Sprintf("signed (%d) should exceed unsigned (%d)", signed.Size(), unsigned.Size()))
	})

	It("rejects an unsupported architecture", func() {
		out, err := aurora.ContainerRun("auroraboot", "sysext",
			"--arch", "i386",
			"--output", resultDir,
			"e2e-badarch", srcImage,
		)
		Expect(err).To(HaveOccurred(), out)
		Expect(out).To(ContainSubstring("unsupported architecture"), out)
	})
})
