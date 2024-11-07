package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("build-uki", Label("build-uki", "e2e"), func() {
	var resultDir string
	var keysDir string
	var resultFile string
	var image string
	var err error
	var auroraboot *Auroraboot

	BeforeEach(func() {
		kairosVersion := "v2.5.0"
		resultDir, err = os.MkdirTemp("", "auroraboot-build-uki-test-")
		Expect(err).ToNot(HaveOccurred())
		resultFile = filepath.Join(resultDir, fmt.Sprintf("kairos_%s.iso", kairosVersion))

		currentDir, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		keysDir = filepath.Join(currentDir, "assets", "keys")
		Expect(os.MkdirAll(keysDir, 0755)).ToNot(HaveOccurred())

		auroraboot = NewAuroraboot("quay.io/kairos/osbuilder-tools", resultDir, keysDir)
		image = fmt.Sprintf("quay.io/kairos/fedora:38-core-amd64-generic-%s", kairosVersion)
	})

	AfterEach(func() {
		os.RemoveAll(resultDir)
		auroraboot.Cleanup()
	})

	When("some dependency is missing", func() {
		BeforeEach(func() {
			auroraboot = NewAuroraboot("busybox", resultDir, keysDir)
		})

		It("returns an error about missing deps", func() {
			out, err := auroraboot.Run("build-uki", "--output-dir", resultDir, "-k", keysDir, "--output-type", "iso", image)
			Expect(err).To(HaveOccurred(), out)
			Expect(out).To(Or(
				MatchRegexp("executable file not found in \\$PATH"),
				MatchRegexp("no such file or directory"),
			))
		})
	})

	Describe("single-efi-cmdline", func() {
		BeforeEach(func() {
			By("building the iso with single-efi-cmdline flags set")
			buildISO(auroraboot, image, keysDir, resultDir, resultFile,
				"--single-efi-cmdline", "My Entry: someoption=somevalue",
				"--single-efi-cmdline", "My Other Entry: someoption2=somevalue2")
		})

		It("creates additional .efi and .conf files", func() {
			content := listEfiFiles(auroraboot, resultFile)
			Expect(string(content)).To(MatchRegexp("my_entry.efi"))
			Expect(string(content)).To(MatchRegexp("my_other_entry.efi"))

			content = listConfFiles(auroraboot, resultFile)
			Expect(string(content)).To(MatchRegexp("my_entry.conf"))
			Expect(string(content)).To(MatchRegexp("my_other_entry.conf"))
		})
	})

	Describe("secure-boot-enroll setting in loader.conf", func() {
		When("secure-boot-enroll is not set", func() {
			BeforeEach(func() {
				By("building the iso with secure-boot-enroll not set")
				buildISO(auroraboot, image, keysDir, resultDir, resultFile)
			})

			It("sets the secure-boot-enroll correctly", func() {
				By("checking if the default value for secure-boot-enroll is set")
				content := readLoaderConf(auroraboot, resultFile)
				Expect(string(content)).To(MatchRegexp("secure-boot-enroll if-safe"))
			})
		})

		When("secure-boot-enroll is set", func() {
			BeforeEach(func() {
				By("building the iso with secure-boot-enroll set to manual")
				buildISO(auroraboot, image, keysDir, resultDir, resultFile, "--secure-boot-enroll", "manual")
			})

			It("sets the secure-boot-enroll correctly", func() {
				By("checking if the user value for secure-boot-enroll is set")
				content := readLoaderConf(auroraboot, resultFile)
				Expect(string(content)).To(MatchRegexp("secure-boot-enroll manual"))
			})
		})
	})
})

func buildISO(auroraboot *Auroraboot, image, keysDir, resultDir, resultFile string, additionalArgs ...string) string {
	args := []string{"build-uki", "--output-dir", resultDir, "-k", keysDir, "--output-type", "iso"}
	args = append(args, additionalArgs...)
	args = append(args, image)
	out, err := auroraboot.Run(args...)
	Expect(err).ToNot(HaveOccurred(), out)

	By("building the iso")
	_, err = os.Stat(resultFile)
	Expect(err).ToNot(HaveOccurred())

	return out
}

func readLoaderConf(auroraboot *Auroraboot, isoFile string) string {
	return runCommandInIso(auroraboot, isoFile, "cat /tmp/efi/loader/loader.conf")
}

func listEfiFiles(auroraboot *Auroraboot, isoFile string) string {
	return runCommandInIso(auroraboot, isoFile, "ls /tmp/efi/EFI/kairos")
}

func listConfFiles(auroraboot *Auroraboot, isoFile string) string {
	return runCommandInIso(auroraboot, isoFile, "ls /tmp/efi/loader/entries")
}

func runCommandInIso(auroraboot *Auroraboot, isoFile, command string) string {
	By("running command: " + command)
	out, err := auroraboot.ContainerRun("/bin/bash", "-c",
		fmt.Sprintf(`#!/bin/bash
set -e
mkdir -p /tmp/iso /tmp/efi
mount -v -o loop %[1]s /tmp/iso 2>&1 > /dev/null
mount -v -o loop /tmp/iso/efiboot.img /tmp/efi 2>&1 > /dev/null
%[2]s
umount /tmp/efi 2>&1 > /dev/null
umount /tmp/iso 2>&1 > /dev/null
`, isoFile, command))
	Expect(err).ToNot(HaveOccurred(), out)

	return out
}
