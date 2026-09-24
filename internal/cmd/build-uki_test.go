package cmd_test

import (
	"bytes"

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
})
