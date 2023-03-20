package auroraboot_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Disk image generation", Label("raw-disks"), func() {

	Context("build from an ISO", func() {

		tempDir := ""

		BeforeEach(func() {
			t, err := os.MkdirTemp("", "")
			Expect(err).ToNot(HaveOccurred())

			tempDir = t

			err = WriteConfig("", t)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tempDir)
		})

		It("generate a raw file", func() {
			out, err := RunAurora(`--set "disable_http_server=true" \
			--set "artifact_version=v1.5.0" \
			--set "release_version=v1.5.0" \
			--set "flavor=rockylinux" \
			--set "disable_netboot=true" \
			--set repository="kairos-io/kairos" \
			--cloud-config /config.yaml \
			--set "disk.raw=true" \
			--set "state_dir=/tmp/auroraboot"`, tempDir)
			Expect(out).To(ContainSubstring("Generating raw disk"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(out).To(ContainSubstring("gen-raw-disk"), out)
			Expect(out).To(ContainSubstring("download-squashfs"), out)
			Expect(out).To(ContainSubstring("extract-squashfs"), out)
			Expect(out).ToNot(ContainSubstring("container-pull"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "build/iso/disk.raw"))
			Expect(err).ToNot(HaveOccurred())
		})

		It("generates a gce image", func() {
			out, err := RunAurora(`--set "disable_http_server=true" \
			--set "disable_netboot=true" \
			--cloud-config /config.yaml \
			--set "artifact_version=v1.5.0" \
			--set repository="kairos-io/kairos" \
			--set "release_version=v1.5.0" \
			--set "flavor=rockylinux" \
			--set "disk.gce=true" \
			--set "state_dir=/tmp/auroraboot"`, tempDir)
			Expect(out).To(ContainSubstring("Generating raw disk"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(out).To(ContainSubstring("gen-raw-disk"), out)
			Expect(out).To(ContainSubstring("download-squashfs"), out)
			Expect(out).To(ContainSubstring("extract-squashfs"), out)
			Expect(out).ToNot(ContainSubstring("container-pull"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "build/iso/disk.raw.gce"))
			Expect(err).ToNot(HaveOccurred())
		})

		It("generates a vhd image", func() {
			out, err := RunAurora(`--set "disable_http_server=true" \
			--set "artifact_version=v1.5.0" \
			--set "release_version=v1.5.0" \
			--set "flavor=rockylinux" \
			--set repository="kairos-io/kairos" \
			--set "disable_netboot=true" \
			--cloud-config /config.yaml \
			--set "disk.vhd=true" \
			--set "state_dir=/tmp/auroraboot"`, tempDir)
			Expect(out).To(ContainSubstring("Generating raw disk"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(out).To(ContainSubstring("gen-raw-disk"), out)
			Expect(out).To(ContainSubstring("download-squashfs"), out)
			Expect(out).To(ContainSubstring("extract-squashfs"), out)
			Expect(out).ToNot(ContainSubstring("container-pull"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "build/iso/disk.raw.vhd"))
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("build from a container image", func() {

		tempDir := ""

		BeforeEach(func() {
			t, err := os.MkdirTemp("", "")
			Expect(err).ToNot(HaveOccurred())

			tempDir = t

			err = WriteConfig("", t)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tempDir)
		})

		It("generate a raw  file", func() {
			image := "quay.io/kairos/core-rockylinux:latest"
			_, err := PullImage(image)
			Expect(err).ToNot(HaveOccurred())

			out, err := RunAurora(fmt.Sprintf(`--set container_image=docker://%s \
			--set "disable_http_server=true" \
			--set "disable_netboot=true" \
			--cloud-config /config.yaml \
			--set "disk.raw=true" \
			--set "state_dir=/tmp/auroraboot"`, image), tempDir)
			Expect(out).To(ContainSubstring("Generating raw disk"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(out).To(ContainSubstring("gen-raw-disk"), out)
			Expect(out).To(ContainSubstring("container-pull"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "build/iso/disk.raw"))
			Expect(err).ToNot(HaveOccurred())
		})

		It("generates a gce image", func() {
			image := "quay.io/kairos/core-opensuse-leap-arm-rpi:latest"
			_, err := PullImage(image)
			Expect(err).ToNot(HaveOccurred())

			out, err := RunAurora(fmt.Sprintf(`--set container_image=docker://%s \
			--set "disable_http_server=true" \
			--set "disable_netboot=true" \
			--cloud-config /config.yaml \
			--set "disk.gce=true" \
			--set "state_dir=/tmp/auroraboot"`, image), tempDir)
			Expect(out).To(ContainSubstring("Generating raw disk"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(out).To(ContainSubstring("gen-raw-disk"), out)
			Expect(out).To(ContainSubstring("convert-gce"), out)
			Expect(out).To(ContainSubstring("container-pull"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "build/iso/disk.raw.gce"))
			Expect(err).ToNot(HaveOccurred())
		})

		It("generates a vhd image", func() {
			image := "quay.io/kairos/core-opensuse-leap-arm-rpi:latest"
			_, err := PullImage(image)
			Expect(err).ToNot(HaveOccurred())

			out, err := RunAurora(fmt.Sprintf(`--set container_image=docker://%s \
			--set "disable_http_server=true" \
			--set "disable_netboot=true" \
			--cloud-config /config.yaml \
			--set "disk.vhd=true" \
			--set "state_dir=/tmp/auroraboot"`, image), tempDir)
			Expect(out).To(ContainSubstring("Generating raw disk"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(out).To(ContainSubstring("gen-raw-disk"), out)
			Expect(out).To(ContainSubstring("convert-vhd"), out)
			Expect(out).To(ContainSubstring("container-pull"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "build/iso/disk.raw.vhd"))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
