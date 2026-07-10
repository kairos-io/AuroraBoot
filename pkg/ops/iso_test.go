package ops

import (
	sdkutils "github.com/kairos-io/kairos-sdk/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("applyGrubTemplate", Label("iso"), func() {
	const templateWithPlaceholders = "linux ($root)/boot/kernel cdroot root=live:CDLABEL=COS_LIVE{{NOMODESET}} install-mode\nlinux ($root)/boot/kernel cdroot{{EXTEND_CMDLINE}}\n"

	It("replaces NOMODESET and EXTEND_CMDLINE with provided values", func() {
		result := applyGrubTemplate([]byte(templateWithPlaceholders), " nomodeset", " rd.debug rd.shell")
		Expect(string(result)).To(ContainSubstring(" nomodeset"))
		Expect(string(result)).To(ContainSubstring(" rd.debug rd.shell"))
		Expect(string(result)).ToNot(ContainSubstring("{{NOMODESET}}"))
		Expect(string(result)).ToNot(ContainSubstring("{{EXTEND_CMDLINE}}"))
	})

	It("replaces EXTEND_CMDLINE with empty string when not provided", func() {
		result := applyGrubTemplate([]byte(templateWithPlaceholders), "", "")
		Expect(string(result)).ToNot(ContainSubstring("{{EXTEND_CMDLINE}}"))
		Expect(string(result)).To(ContainSubstring("install-mode\nlinux ($root)/boot/kernel cdroot\n"))
	})

	It("replaces NOMODESET with empty string when not provided", func() {
		result := applyGrubTemplate([]byte(templateWithPlaceholders), "", " rd.debug")
		Expect(string(result)).ToNot(ContainSubstring("{{NOMODESET}}"))
		Expect(string(result)).To(ContainSubstring(" rd.debug"))
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
