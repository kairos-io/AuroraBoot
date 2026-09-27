package cmd_test

import (
	"bytes"
	"os"

	cmdpkg "github.com/kairos-io/AuroraBoot/internal/cmd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v2"
)

var _ = Describe("build-uki", Label("uki", "cmd"), func() {
	var app *cli.App
	var err error
	var buf *bytes.Buffer

	BeforeEach(func() {
		buf = new(bytes.Buffer)
		app = cmdpkg.GetApp("v0.0.0")
		app.Writer = buf
	})

	It("Accepts the allow-insecure-registries flag", Label("flags"), func() {
		err = app.Run([]string{"", "build-uki", "--allow-insecure-registries", "--public-keys", "/tmp", "some/image:latest"})
		// Fails later in the build, but the flag must be accepted (not rejected at parse time).
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).ToNot(ContainSubstring("flag provided but not defined"))
	})

	It("accepts repeatable extension flags", Label("flags"), func() {
		err = app.Run([]string{"", "build-uki", "--tpm-pcr-private-key", "pcr.key", "--sb-key", "sb.key", "--sb-cert", "sb.pem", "--extension", "tool", "--extension", "debug@v2", "--extensions-catalog", "catalog.yaml", "some/image:latest"})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).ToNot(ContainSubstring("flag provided but not defined"))
	})

	It("accepts an extension with no catalog, which reads the default one", Label("flags"), func() {
		err = app.Run([]string{"", "build-uki", "--tpm-pcr-private-key", "pcr.key", "--sb-key", "sb.key", "--sb-cert", "sb.pem", "--extension", "tool", "some/image:latest"})
		// Fails later in the build (this is not root, and there is no such
		// image), but no longer on the missing catalog.
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).ToNot(ContainSubstring("extensions-catalog is required"))
	})

	It("rejects malformed extension requests", Label("flags"), func() {
		err = app.Run([]string{"", "build-uki", "--tpm-pcr-private-key", "pcr.key", "--sb-key", "sb.key", "--sb-cert", "sb.pem", "--extension", "tool@", "--extensions-catalog", "catalog.yaml", "some/image:latest"})
		Expect(err).To(MatchError(ContainSubstring("invalid extension request")))
	})

	It("rejects a missing required flag before the root check", Label("flags"), func() {
		err = app.Run([]string{"", "build-uki", "--sb-key", "/tmp/sb.key", "--sb-cert", "/tmp/sb.pem", "some/image:latest"})

		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("Required flag \"tpm-pcr-private-key\" not set"))
		Expect(err.Error()).ToNot(ContainSubstring("this command requires root privileges"))
		Expect(err.Error()).ToNot(ContainSubstring("flag provided but not defined"))
	})

	It("accepts the app-level --cloud-config flag before the subcommand", Label("flags"), func() {
		cc := GinkgoT().TempDir() + "/cc.yaml"
		Expect(os.WriteFile(cc, []byte("#cloud-config\nusers:\n  - name: kairos\n"), 0o644)).To(Succeed())

		err = app.Run([]string{"", "--cloud-config", cc, "build-uki", "some/image:latest"})

		Expect(err).ToNot(BeNil())
		Expect(err.Error()).ToNot(ContainSubstring("flag provided but not defined"))
		Expect(err.Error()).ToNot(ContainSubstring("unknown flag"))
	})

	It("accepts the subcommand --cloud-config flag after build-uki", Label("flags"), func() {
		cc := GinkgoT().TempDir() + "/cc.yaml"
		Expect(os.WriteFile(cc, []byte("#cloud-config\nusers:\n  - name: kairos\n"), 0o644)).To(Succeed())

		err = app.Run([]string{"", "build-uki", "--cloud-config", cc, "some/image:latest"})

		Expect(err).ToNot(BeNil())
		Expect(err.Error()).ToNot(ContainSubstring("flag provided but not defined"))
		Expect(err.Error()).ToNot(ContainSubstring("unknown flag"))
	})

	It("passes the post-subcommand --cloud-config value to ReadCloudConfig", Label("flags"), func() {
		if os.Geteuid() != 0 {
			Skip("the build-uki Action (and thus the cloud-config read) only runs as root; run this test as root to exercise the wiring")
		}

		missing := "/nonexistent/cc.yaml"
		err = app.Run([]string{
			"",
			"build-uki",
			"--tpm-pcr-private-key", "/nonexistent/key.pem",
			"--sb-key", "/nonexistent/sb.key",
			"--sb-cert", "/nonexistent/sb.pem",
			"--cloud-config", missing,
			"some/image:latest",
		})

		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("reading cloud config"))
		Expect(err.Error()).To(ContainSubstring("file '" + missing + "' not found"))
	})
})
