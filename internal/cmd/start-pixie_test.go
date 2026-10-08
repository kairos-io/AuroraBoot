package cmd_test

import (
	"bytes"
	"context"
	"time"

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
		// cloud-config-file is not one of ValidateStartPixieArgs' parameters at
		// all, so there is nothing to pass for it.
		err := cmdpkg.ValidateStartPixieArgs("rootfs.squashfs", "127.0.0.1", "0", "initrd.img", "vmlinuz")
		Expect(err).To(BeNil())
	})

	It("returns when its context is done", func() {
		// The whole command, not just its argument check: Action binds real
		// sockets and the server blocks until a fatal error or a shutdown, so
		// the command has to shut the server down when the context ends. This
		// is what hung CI for two hours.
		//
		// Which error comes back depends on the privileges: a run that can
		// bind all four listeners reaches the deadline, an unprivileged one
		// fails the DHCP bind first. Returning at all is the property here.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- app.RunContext(ctx, []string{"", "start-pixie", "", "rootfs.squashfs", "127.0.0.1", "0", "initrd.img", "vmlinuz"})
		}()

		Eventually(done, 30*time.Second).Should(Receive())
	})

	It("shows help output", func() {
		err = app.Run([]string{"", "start-pixie", "--help"})
		Expect(err).To(BeNil())
		Expect(buf.String()).To(ContainSubstring("Start the Pixiecore netboot server"))
	})
})
