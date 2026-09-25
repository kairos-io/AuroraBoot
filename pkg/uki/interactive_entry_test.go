package uki

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kairos-io/AuroraBoot/pkg/constants"
	"github.com/kairos-io/AuroraBoot/pkg/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// plainInstallerGuard is the cmdline test the bundled 52_installer.yaml runs
// to decide whether to start the unattended installer. install-mode is a
// prefix of install-mode-interactive, so it is matched as a whole word.
var plainInstallerGuard = regexp.MustCompile(`(^| )install-mode( |$)`)

func entryTitled(entries []utils.BootEntry, title string) utils.BootEntry {
	GinkgoHelper()
	for _, e := range entries {
		if e.Title == title {
			return e
		}
	}
	Fail("no boot entry titled " + title)
	return utils.BootEntry{}
}

var _ = Describe("GetUkiCmdline interactive install entry", func() {
	It("offers an interactive install entry next to the unattended one", func() {
		entries := GetUkiCmdline("", "Kairos", nil, false)

		Expect(entries).To(HaveLen(2))
		Expect(entries[0].Title).To(Equal("Kairos"))
		Expect(entries[0].Cmdline).To(HaveSuffix(" " + constants.UkiCmdlineInstall))

		interactive := entries[1]
		Expect(interactive.Title).To(Equal("Kairos (interactive install)"))
		Expect(interactive.Cmdline).To(Equal(constants.UkiCmdline + " " + constants.UkiCmdlineInstallInteractive))
		Expect(interactive.FileName).To(Equal(constants.ArtifactBaseName + "_install-mode-interactive"))
	})

	It("keeps the two entries on the boot-time guards that tell them apart", func() {
		entries := GetUkiCmdline("", "Kairos", nil, false)
		unattended := entries[0].Cmdline
		interactive := entries[1].Cmdline

		// The unattended entry starts the plain installer and not the TUI.
		Expect(plainInstallerGuard.MatchString(unattended)).To(BeTrue())
		Expect(unattended).NotTo(ContainSubstring(constants.UkiCmdlineInstallInteractive))

		// The interactive entry starts the TUI and not the plain installer.
		Expect(interactive).To(ContainSubstring(constants.UkiCmdlineInstallInteractive))
		Expect(plainInstallerGuard.MatchString(interactive)).To(BeFalse())
	})

	It("carries the interactive entry through extend mode too", func() {
		entries := GetUkiCmdline("foo=bar", "Kairos", nil, false)

		Expect(entries).To(HaveLen(2))
		Expect(entries[0].Cmdline).To(HaveSuffix(" foo=bar"))

		interactive := entries[1]
		Expect(interactive.Cmdline).To(Equal(constants.UkiCmdline + " " + constants.UkiCmdlineInstallInteractive + " foo=bar"))
		// The name is derived from the bare interactive cmdline, so extending
		// does not rename the EFI file.
		Expect(interactive.FileName).To(Equal(constants.ArtifactBaseName + "_install-mode-interactive"))
		Expect(plainInstallerGuard.MatchString(interactive.Cmdline)).To(BeFalse())
	})

	It("still produces one entry per extra cmdline", func() {
		entries := GetUkiCmdline("", "Kairos", []string{"role=worker", "role=control"}, false)

		Expect(entries).To(HaveLen(4))
		titles := []string{}
		for _, e := range entries {
			titles = append(titles, e.Title)
		}
		Expect(titles).To(ContainElement("Kairos (interactive install)"))

		worker := entries[2]
		Expect(worker.Cmdline).To(HaveSuffix(" role=worker"))
		Expect(plainInstallerGuard.MatchString(worker.Cmdline)).To(BeTrue())
	})

	It("does not add extra entries in cmd-lines-v2 mode, where extras are profiles", func() {
		entries := GetUkiCmdline("", "Kairos", []string{"role=worker"}, true)

		Expect(entries).To(HaveLen(2))
		Expect(entries[1].Title).To(Equal("Kairos (interactive install)"))
	})

	It("leaves the unattended entry as the one systemd-boot selects", func() {
		dir := GinkgoT().TempDir()
		entries := GetUkiCmdline("", "Kairos", nil, false)

		for _, e := range entries {
			Expect(createConfFiles(dir, e.Cmdline, e.Title, e.FileName, "v1.0.0", "0", false, true)).To(Succeed())
		}

		// systemd-boot orders the menu by sort-key, and with no `default` in
		// loader.conf the first one wins. Read the keys back out of the
		// generated conf files rather than recomputing them.
		keys := map[string]string{}
		for _, e := range entries {
			data, err := os.ReadFile(filepath.Join(dir, "entries", e.FileName+".conf"))
			Expect(err).ToNot(HaveOccurred())
			for _, line := range strings.Split(string(data), "\n") {
				if after, found := strings.CutPrefix(line, "sort-key "); found {
					keys[e.Title] = after
				}
			}
		}
		Expect(keys).To(HaveLen(2))

		sorted := []string{keys["Kairos"], keys["Kairos (interactive install)"]}
		Expect(sort.StringsAreSorted(sorted)).To(BeTrue(),
			"the unattended entry must still sort first, got %v", sorted)
	})

	It("writes the interactive cmdline into the entry's conf file", func() {
		dir := GinkgoT().TempDir()
		interactive := entryTitled(GetUkiCmdline("", "Kairos", nil, false), "Kairos (interactive install)")

		Expect(createConfFiles(dir, interactive.Cmdline, interactive.Title, interactive.FileName, "v1.0.0", "0", false, true)).To(Succeed())

		data, err := os.ReadFile(filepath.Join(dir, "entries", interactive.FileName+".conf"))
		Expect(err).ToNot(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("cmdline " + constants.UkiCmdlineInstallInteractive))
		Expect(string(data)).To(ContainSubstring("uki /EFI/kairos/" + interactive.FileName + ".efi"))
	})
})
