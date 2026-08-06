package ops

import (
	"path/filepath"
	"strings"

	sdkConfig "github.com/kairos-io/kairos-sdk/types/config"
	"github.com/kairos-io/kairos-sdk/types/logger"

	"github.com/kairos-io/AuroraBoot/pkg/constants"
	sdkutils "github.com/kairos-io/kairos-sdk/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/twpayne/go-vfs/v5/vfst"
)

var _ = Describe("applyGrubTemplate", Label("iso"), func() {
	const templateWithPlaceholders = "linux ($root)/boot/kernel cdroot root=live:CDLABEL=COS_LIVE {{LIVE_CONSOLE}}{{NOMODESET}} install-mode\nlinux ($root)/boot/kernel cdroot{{EXTEND_CMDLINE}}\nmenuentry debug { linux console=tty0 }\n"

	It("replaces NOMODESET and EXTEND_CMDLINE with provided values", func() {
		result := applyGrubTemplate([]byte(templateWithPlaceholders), " nomodeset", " rd.debug rd.shell", "")
		Expect(string(result)).To(ContainSubstring(" nomodeset"))
		Expect(string(result)).To(ContainSubstring(" rd.debug rd.shell"))
		Expect(string(result)).ToNot(ContainSubstring("{{NOMODESET}}"))
		Expect(string(result)).ToNot(ContainSubstring("{{EXTEND_CMDLINE}}"))
	})

	It("replaces EXTEND_CMDLINE with empty string when not provided", func() {
		result := applyGrubTemplate([]byte(templateWithPlaceholders), "", "", "")
		Expect(string(result)).ToNot(ContainSubstring("{{EXTEND_CMDLINE}}"))
		Expect(string(result)).To(ContainSubstring("install-mode\nlinux ($root)/boot/kernel cdroot\n"))
	})

	It("replaces NOMODESET with empty string when not provided", func() {
		result := applyGrubTemplate([]byte(templateWithPlaceholders), "", " rd.debug", "")
		Expect(string(result)).ToNot(ContainSubstring("{{NOMODESET}}"))
		Expect(string(result)).To(ContainSubstring(" rd.debug"))
	})

	It("uses the default live consoles when no override is provided", func() {
		result := applyGrubTemplate(constants.GrubLiveBiosCfg, "", "", "")
		Expect(string(result)).To(ContainSubstring("console=ttyS0 console=tty1"))
		Expect(string(result)).ToNot(ContainSubstring("{{LIVE_CONSOLE}}"))
	})

	It("replaces live consoles while preserving the debug console", func() {
		result := applyGrubTemplate(constants.GrubLiveBiosCfg, "", "", "console=ttyUSB0,115200")
		Expect(string(result)).ToNot(ContainSubstring("console=ttyS0 console=tty1"))
		Expect(strings.Count(string(result), "console=ttyUSB0,115200")).To(Equal(6))
		Expect(string(result)).To(ContainSubstring("console=tty0 rd.debug"))
	})

	It("strips carriage returns and newlines from a live console override", func() {
		result := applyGrubTemplate([]byte("linux {{LIVE_CONSOLE}} end"), "", "", "console=ttyS1\r\nconsole=tty1")
		Expect(string(result)).To(Equal("linux console=ttyS1console=tty1 end"))
	})
})

var _ = Describe("getEfiGrubFilesForArch", Label("iso"), func() {
	It("prepends the openSUSE riscv64 path before SDK paths", func() {
		paths := getEfiGrubFilesForArch("riscv64")
		sdkPaths := sdkutils.GetEfiGrubFiles("riscv64")

		Expect(paths[0]).To(Equal("/usr/share/efi/riscv64/grub.efi"))
		Expect(paths).To(Equal(append([]string{"/usr/share/efi/riscv64/grub.efi"}, sdkPaths...)))
	})

	It("prepends gcdx64.efi.signed on amd64 so the ISO uses the CD grub, not the disk one", func() {
		// gcdx64.efi.signed has iso9660 baked in and its prefix set to
		// /boot/grub, which is what a live ISO needs. grubx64.efi.signed
		// is built for disk installs and its /EFI/ubuntu prefix breaks on
		// firmware that sets $root to the ISO9660 partition.
		paths := getEfiGrubFilesForArch("amd64")
		Expect(paths[0]).To(Equal("/usr/lib/grub/x86_64-efi-signed/gcdx64.efi.signed"))
		Expect(paths).To(ContainElement("/usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed"))
		Expect(indexOf(paths, "/usr/lib/grub/x86_64-efi-signed/gcdx64.efi.signed")).To(
			BeNumerically("<", indexOf(paths, "/usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed")),
		)
	})

	It("prepends gcdaa64.efi.signed on arm64 for the same reason", func() {
		paths := getEfiGrubFilesForArch("arm64")
		Expect(paths[0]).To(Equal("/usr/lib/grub/arm64-efi-signed/gcdaa64.efi.signed"))
		Expect(paths).To(ContainElement("/usr/lib/grub/arm64-efi-signed/grubaa64.efi.signed"))
		Expect(indexOf(paths, "/usr/lib/grub/arm64-efi-signed/gcdaa64.efi.signed")).To(
			BeNumerically("<", indexOf(paths, "/usr/lib/grub/arm64-efi-signed/grubaa64.efi.signed")),
		)
	})

	It("keeps the full SDK path list after the CD-grub prepends", func() {
		amd64Paths := getEfiGrubFilesForArch("amd64")
		for _, p := range sdkutils.GetEfiGrubFiles("amd64") {
			Expect(amd64Paths).To(ContainElement(p))
		}
	})
})

func indexOf(list []string, target string) int {
	for i, s := range list {
		if s == target {
			return i
		}
	}
	return -1
}

var _ = Describe("writeCdGrubEfiCfg", Label("iso"), func() {
	// gcdx64.efi.signed (and its arm64 equivalent) is compiled with prefix
	// /boot/grub. When the CD-media grub is loaded it looks for its config at
	// ($root)/boot/grub/grub.cfg. Without a file at that path the ISO drops
	// to the grub rescue prompt on firmwares where $root is the ISO device.
	It("writes grub.cfg at /boot/grub/grub.cfg with the chainloader stub", func() {
		fs, cleanup, err := vfst.NewTestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanup)

		b := &BuildISOAction{cfg: &BuildConfig{Config: sdkConfig.Config{
			Fs:     fs,
			Logger: logger.NewNullLogger(),
		}}}

		root := "/iso"
		Expect(fs.Mkdir(root, 0o755)).To(Succeed())

		Expect(b.writeCdGrubEfiCfg(root)).To(Succeed())

		out, err := fs.ReadFile(filepath.Join(root, "boot", "grub", constants.GrubCfg))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(Equal(constants.GrubEfiCfg))
	})
})

var _ = Describe("cleanupGrubName", Label("iso"), func() {
	DescribeTable("strips signature suffixes",
		func(in, want string) {
			Expect(cleanupGrubName(in)).To(Equal(want))
		},
		Entry("plain grubx64.efi", "grubx64.efi", "grubx64.efi"),
		Entry("Ubuntu grubx64.efi.signed", "grubx64.efi.signed", "grubx64.efi"),
		Entry("Ubuntu grubaa64.efi.signed", "grubaa64.efi.signed", "grubaa64.efi"),
		Entry("Ubuntu grubaa64.efi.dualsigned", "grubaa64.efi.dualsigned", "grubaa64.efi"),
		Entry("Ubuntu grubaa64.efi.signed.latest", "grubaa64.efi.signed.latest", "grubaa64.efi"),
	)

	DescribeTable("renames the CD grub to the name shim chainloads",
		func(in, want string) {
			// Ubuntu's shim is compiled to load grub{x64,aa64}.efi from the
			// same directory. When we ship the CD variant of grub, its
			// filename is gcd*.efi.signed. We must rename it so shim finds
			// it under Secure Boot.
			Expect(cleanupGrubName(in)).To(Equal(want))
		},
		Entry("gcdx64.efi.signed becomes grubx64.efi", "gcdx64.efi.signed", "grubx64.efi"),
		Entry("gcdaa64.efi.signed becomes grubaa64.efi", "gcdaa64.efi.signed", "grubaa64.efi"),
		Entry("plain gcdx64.efi (unsigned build) also renames", "gcdx64.efi", "grubx64.efi"),
	)
})
