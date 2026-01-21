package cmd_test

import (
	"bytes"

	cmdpkg "github.com/kairos-io/AuroraBoot/internal/cmd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/cli/v2"
)

var _ = Describe("build-iso", Label("iso", "cmd"), func() {
	var app *cli.App
	var err error
	var buf *bytes.Buffer

	BeforeEach(func() {
		buf = new(bytes.Buffer)

		app = cmdpkg.GetApp("v0.0.0")
		app.Writer = buf
	})

	It("errors out if no rootfs sources are defined", func() {
		err = app.Run([]string{"", "build-iso"}) // first arg is the path to the program
		Expect(err.Error()).To(Equal("no source defined"))
	})

	It("Errors out if rootfs is a non valid argument", Label("flags"), func() {
		err = app.Run([]string{"", "build-iso", "/no/image/reference"})
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("invalid image reference"))
	})

	It("Errors out if overlay roofs path does not exist", Label("flags"), func() {
		err = app.Run([]string{"", "build-iso", "--overlay-rootfs", "/nonexistingpath", "system/cos"})
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("invalid path"))
	})

	It("Errors out if overlay uefi path does not exist", Label("flags"), func() {
		err = app.Run([]string{"", "build-iso", "--overlay-uefi", "/nonexistingpath", "someimage:latest"})
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("invalid path"))
	})

	It("Errors out if overlay iso path does not exist", Label("flags"), func() {
		err = app.Run([]string{"", "build-iso", "--overlay-iso", "/nonexistingpath", "some/image:latest"})
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("invalid path"))
	})

	It("Errors out if arch is invalid", Label("flags"), func() {
		err = app.Run([]string{"", "build-iso", "--arch", "invalid", "some/image:latest"})
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).To(ContainSubstring("invalid architecture"))
		Expect(err.Error()).To(ContainSubstring("must be 'amd64' or 'arm64'"))
	})

	It("Accepts amd64 as a valid arch", Label("flags"), func() {
		err = app.Run([]string{"", "build-iso", "--arch", "amd64", "system/cos"})
		// This will still error out because system/cos is not a valid image reference,
		// but it should not error on the arch validation
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).ToNot(ContainSubstring("invalid architecture"))
	})

	It("Accepts arm64 as a valid arch", Label("flags"), func() {
		err = app.Run([]string{"", "build-iso", "--arch", "arm64", "system/cos"})
		// This will still error out because system/cos is not a valid image reference,
		// but it should not error on the arch validation
		Expect(err).ToNot(BeNil())
		Expect(err.Error()).ToNot(ContainSubstring("invalid architecture"))
	})
})
