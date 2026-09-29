package uki

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kairos-io/AuroraBoot/pkg/constants"
	sdkLogger "github.com/kairos-io/kairos/v4/sdk/types/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// installKeywords are the keywords that ask a Kairos boot to run an installer.
// None of them may reach a UKI cmdline: on Trusted Boot the cmdline lives in a
// signed section of the EFI and the installer copies norole.efi byte for byte
// into the active, passive, recovery and statereset roles, so a keyword set for
// the live medium stays on the installed machine's kernel command line for the
// life of the machine. kairos-io/kairos#5000.
var installKeywords = []string{"install-mode", "install-mode-interactive", "interactive-install"}

func cmdlineHasKeyword(cmdline, keyword string) bool {
	for _, arg := range strings.Fields(cmdline) {
		if arg == keyword {
			return true
		}
	}
	return false
}

var _ = Describe("UKI cmdline", func() {
	Describe("GetUkiCmdline", func() {
		It("builds the default entry from the base cmdline alone", func() {
			entries := GetUkiCmdline("", "Kairos", nil, false)
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Cmdline).To(Equal(constants.UkiCmdline))
			Expect(entries[0].FileName).To(Equal(constants.ArtifactBaseName))
		})

		It("puts no install keyword on any entry it builds", func() {
			entries := GetUkiCmdline("extended=1", "Kairos", []string{"extra=1"}, false)
			entries = append(entries, GetUkiCmdline("", "Kairos", []string{"extra=1"}, false)...)
			entries = append(entries, GetUkiSingleCmdlines("Kairos", []string{"My Entry: single=1", "bare=1"}, sdkLogger.NewNullLogger())...)

			Expect(entries).ToNot(BeEmpty())
			for _, entry := range entries {
				for _, keyword := range installKeywords {
					Expect(cmdlineHasKeyword(entry.Cmdline, keyword)).To(BeFalse(),
						"entry %q must not carry %q, got %q", entry.FileName, keyword, entry.Cmdline)
				}
			}
		})

		// An entry whose whole addition is "install-mode" used to be renamed to
		// the bare basename, which is the default entry's name. Two entries with
		// one name overwrite each other's .efi and .conf.
		It("gives every entry its own file name, install keyword or not", func() {
			entries := GetUkiCmdline("", "Kairos", []string{"install-mode", "install-mode-interactive"}, false)

			names := []string{}
			for _, entry := range entries {
				names = append(names, entry.FileName)
			}
			// ConsistOf compares as a multiset, so a name claimed twice fails
			// here rather than silently overwriting an .efi on disk.
			Expect(names).To(ConsistOf(
				constants.ArtifactBaseName,
				constants.ArtifactBaseName+"_install-mode",
				constants.ArtifactBaseName+"_install-mode-interactive",
			))
		})
	})

	Describe("createConfFiles", func() {
		var sourceDir string

		BeforeEach(func() {
			sourceDir = GinkgoT().TempDir()
		})

		readConf := func(name string) string {
			content, err := os.ReadFile(filepath.Join(sourceDir, "entries", name))
			Expect(err).ToNot(HaveOccurred())
			return string(content)
		}

		It("writes an empty cmdline line for the default entry, which adds nothing", func() {
			Expect(createConfFiles(sourceDir, constants.UkiCmdline, "Kairos", constants.ArtifactBaseName, "v1", "0", false, true)).To(Succeed())
			Expect(readConf(constants.ArtifactBaseName + ".conf")).To(ContainSubstring("cmdline \n"))
		})

		It("records what a non-default entry adds, including an install keyword", func() {
			Expect(createConfFiles(sourceDir, constants.UkiCmdline+" install-mode", "Kairos", constants.ArtifactBaseName+"_install-mode", "v1", "0", false, true)).To(Succeed())
			Expect(readConf(constants.ArtifactBaseName + "_install-mode.conf")).To(ContainSubstring("cmdline install-mode\n"))
		})
	})
})
