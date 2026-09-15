package cmd_test

import (
	"bytes"

	cmdpkg "github.com/kairos-io/AuroraBoot/internal/cmd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v2"
)

var _ = Describe("start-pixie", Label("pixie", "cmd"), func() {
	var app *cli.App
	var err error
	var buf *bytes.Buffer

	BeforeEach(func() {
		buf = new(bytes.Buffer)
		app = cmdpkg.GetApp("v0.0.0")
		app.Writer = buf
	})

	It("errors out if no arguments are provided", func() {
		err = app.Run([]string{"", "start-pixie"})
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("all arguments except cloud-config-file are required"))
	})

	It("errors out if only some arguments are provided", func() {
		err = app.Run([]string{"", "start-pixie", "cloud.yaml", "rootfs.squashfs"})
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("all arguments except cloud-config-file are required"))
	})

	It("does not fail validation when cloud-config-file is empty", func() {
		// Exercises ValidateStartPixieArgs directly -- cloud-config-file isn't
		// one of its parameters at all, so there's nothing to pass for it.
		// Deliberately does NOT go through app.Run/RunContext: past that
		// point Action binds a real raw socket and blocks on network I/O,
		// which a context timeout does not reliably interrupt (it didn't on
		// CI, where the bind succeeds and the test hung for two hours).
		err := cmdpkg.ValidateStartPixieArgs("rootfs.squashfs", "127.0.0.1", "0", "initrd.img", "vmlinuz")
		Expect(err).To(BeNil())
	})

	It("shows help output", func() {
		err = app.Run([]string{"", "start-pixie", "--help"})
		Expect(err).To(BeNil())
		Expect(buf.String()).To(ContainSubstring("Start the Pixiecore netboot server"))
	})
})
