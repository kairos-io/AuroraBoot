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
})
