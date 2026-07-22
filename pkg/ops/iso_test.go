package ops

import (
	"strings"

	"github.com/kairos-io/AuroraBoot/pkg/constants"
	sdkutils "github.com/kairos-io/kairos-sdk/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		Expect(strings.Count(string(result), "console=ttyUSB0,115200")).To(Equal(5))
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

	It("returns the SDK path list for non-riscv64 arches", func() {
		Expect(getEfiGrubFilesForArch("arm64")).To(Equal(sdkutils.GetEfiGrubFiles("arm64")))
	})
})
