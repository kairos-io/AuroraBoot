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

	It("accepts the app-level --cloud-config flag before the subcommand", Label("flags"), func() {
		cc := GinkgoT().TempDir() + "/cc.yaml"
		Expect(os.WriteFile(cc, []byte("#cloud-config\nusers:\n  - name: kairos\n"), 0o644)).To(Succeed())

		err = app.Run([]string{"", "--cloud-config", cc, "build-uki", "some/image:latest"})

		Expect(err).ToNot(BeNil())
		Expect(err.Error()).ToNot(ContainSubstring("flag provided but not defined"))
		Expect(err.Error()).ToNot(ContainSubstring("unknown flag"))
	})
})
