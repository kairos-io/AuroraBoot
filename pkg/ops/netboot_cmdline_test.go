package ops

import (
	"github.com/kairos-io/AuroraBoot/pkg/schema"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("netbootCmdline", func() {
	const (
		squashFS = "/artifacts/kairos.squashfs"
		config   = "/artifacts/config.yaml"
	)

	Describe("the default cmdline", func() {
		var cmdline string

		BeforeEach(func() {
			cmdline = netbootCmdline(squashFS, config, schema.NetBoot{})
		})

		// Without the keyword 52_installer.yaml writes no
		// kairos-installer.service, and nothing else on a netbooted node runs
		// the install. See kairos-io/kairos#5373.
		It("asks for the installer", func() {
			Expect(cmdline).To(ContainSubstring(" install-mode "))
		})

		// install-mode is a prefix of install-mode-interactive and the guard
		// matches space-delimited, so the plain keyword has to stand alone to
		// select the plain installer rather than the interactive one.
		It("does not ask for the interactive installer", func() {
			Expect(cmdline).NotTo(ContainSubstring("install-mode-interactive"))
			Expect(cmdline).NotTo(ContainSubstring("interactive-install"))
		})

		// kairos-sdk reads the boot state off this keyword, and immucore only
		// writes /run/cos/live_mode when it says LiveCD. install-mode on its
		// own would not be enough.
		It("still marks the boot as a netboot", func() {
			Expect(cmdline).To(ContainSubstring(" netboot "))
		})

		It("serves the rootfs and the config through pixiecore", func() {
			Expect(cmdline).To(ContainSubstring(`root=live:{{ ID "` + squashFS + `" }}`))
			Expect(cmdline).To(ContainSubstring(`config_url={{ ID "` + config + `" }}`))
		})
	})

	Describe("an explicit netboot.cmdline", func() {
		var cmdline string

		BeforeEach(func() {
			cmdline = netbootCmdline(squashFS, config, schema.NetBoot{Cmdline: "kairos.ram console=ttyS0"})
		})

		// The override is how an operator asks for a netbooted node that does
		// not install itself, so the default must not survive underneath it.
		It("replaces the default instead of adding to it", func() {
			Expect(cmdline).To(HaveSuffix(" kairos.ram console=ttyS0"))
			Expect(cmdline).NotTo(ContainSubstring("install-mode"))
			Expect(cmdline).NotTo(ContainSubstring("rd.live.overlay.overlayfs"))
		})

		It("keeps the two keys netboot has to control", func() {
			Expect(cmdline).To(ContainSubstring(`root=live:{{ ID "` + squashFS + `" }}`))
			Expect(cmdline).To(ContainSubstring(`config_url={{ ID "` + config + `" }}`))
		})
	})
})
