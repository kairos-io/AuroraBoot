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
		Expect(err.Error()).To(ContainSubstring("all arguments are required"))
	})

	It("errors out if only some arguments are provided", func() {
		err = app.Run([]string{"", "start-pixie", "cloud.yaml", "rootfs.squashfs"})
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("all arguments are required"))
	})

	It("shows help output", func() {
		err = app.Run([]string{"", "start-pixie", "--help"})
		Expect(err).To(BeNil())
		Expect(buf.String()).To(ContainSubstring("Start the Pixiecore netboot server"))
	})

	// The flag carries the livecd grub config into the netboot cmdline
	// (kairos-io/kairos#2573). All positional arguments are empty, so the
	// action stops at the argument check and starts no server: what is under
	// test is that the flag parses and is wired to the command.
	It("accepts the grub-cfg flag", Label("flags"), func() {
		err = app.Run([]string{"", "start-pixie", "--grub-cfg", "/tmp/kairos-grub.cfg"})
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("all arguments are required"))
		Expect(err.Error()).ToNot(ContainSubstring("grub-cfg"))
		Expect(err.Error()).ToNot(ContainSubstring("flag provided but not defined"))
	})

	// Asserted on the flag's own Usage text, not on the hand-written
	// Description block, which mentions --grub-cfg whatever the flag is
	// really called.
	It("documents the grub-cfg flag in its help", Label("flags"), func() {
		err = app.Run([]string{"", "start-pixie", "--help"})
		Expect(err).To(BeNil())
		Expect(buf.String()).To(ContainSubstring("--grub-cfg value"))
		Expect(buf.String()).To(ContainSubstring("extracted by the netboot command"))
	})
})
