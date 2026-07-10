package ops

import (
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
	It("includes openSUSE and SDK riscv64 grub EFI paths", func() {
		paths := getEfiGrubFilesForArch("riscv64")
		Expect(paths).To(ContainElement("/usr/share/efi/riscv64/grub.efi"))
		Expect(paths).To(ContainElement("/usr/lib/grub/riscv64-efi/monolithic/grubriscv64.efi"))
		Expect(paths).To(ContainElement("/boot/efi/EFI/ubuntu/grubriscv64.efi"))
		Expect(paths).To(ContainElement("/boot/efi/EFI/debian/grubriscv64.efi"))
	})

	It("delegates non-riscv64 arches to the SDK", func() {
		Expect(getEfiGrubFilesForArch("arm64")).To(ContainElement("/usr/lib/grub/arm64-efi/grubaa64.efi"))
	})
})
