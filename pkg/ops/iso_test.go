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
