package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// As this tests all use loop devices, they should be run serially so they dont hit each other while acquiring the loop device number
var _ = Describe("Disk image generation", Label("raw-disks"), Serial, func() {
	var tempDir string
	var err error
	var aurora *Auroraboot

	BeforeEach(func() {
		tempDir, err = os.MkdirTemp("", "auroraboot-test-")
		Expect(err).ToNot(HaveOccurred())

		err = WriteConfig("test", tempDir)
		Expect(err).ToNot(HaveOccurred())

		aurora = NewAuroraboot("auroraboot")
		// Map the config.yaml file to the container and the temp dir to the state dir
		aurora.ManualDirs = map[string]string{
			fmt.Sprintf("%s/config.yaml", tempDir): "/config.yaml",
			tempDir:                                "/tmp/auroraboot",
		}
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
		aurora.Cleanup()
	})

	Context("source is an ISO", func() {
		Describe("EFI", Label("efi"), func() {
			It("generate a raw file", func() {
				out, err := aurora.Run("--debug",
					"--set", "disable_http_server=true",
					"--set", "disable_netboot=true",
					"--set", "artifact_version=v3.2.1",
					"--set", "release_version=v3.2.1",
					"--set", "flavor=rockylinux",
					"--set", "flavor_release=9",
					"--set", "repository=kairos-io/kairos",
					"--set", "state_dir=/tmp/auroraboot",
					"--set", "disk.raw=true",
					"--cloud-config", "/config.yaml",
				)
				Expect(out).To(ContainSubstring("Generating raw disk"), out)
				Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
				Expect(out).To(ContainSubstring("gen-raw-disk"), out)
				Expect(out).To(ContainSubstring("download-squashfs"), out)
				Expect(out).To(ContainSubstring("extract-squashfs"), out)
				Expect(out).ToNot(ContainSubstring("dump-source"), out)
				Expect(err).ToNot(HaveOccurred(), out)
				// should be named: kairos-rockylinux-9-core-amd64-generic-v3.2.1.raw based on the source artifact
				_, err = os.Stat(filepath.Join(tempDir, "kairos-rockylinux-9-core-amd64-generic-v3.2.1.raw"))
				Expect(err).ToNot(HaveOccurred(), out)
			})
			It("generates a gce image", func() {
				out, err := aurora.Run("--debug",
					"--set", "disable_http_server=true",
					"--set", "disable_netboot=true",
					"--set", "artifact_version=v3.2.1",
					"--set", "release_version=v3.2.1",
					"--set", "flavor=rockylinux",
					"--set", "flavor_release=9",
					"--set", "repository=kairos-io/kairos",
					"--set", "disable_netboot=true",
					"--set", "state_dir=/tmp/auroraboot",
					"--set", "disk.gce=true",
					"--cloud-config", "/config.yaml",
				)
				Expect(out).To(ContainSubstring("Generating raw disk"), out)
				Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
				Expect(out).To(ContainSubstring("gen-raw-disk"), out)
				Expect(out).To(ContainSubstring("download-squashfs"), out)
				Expect(out).To(ContainSubstring("extract-squashfs"), out)
				Expect(out).ToNot(ContainSubstring("dump-source"), out)
				Expect(err).ToNot(HaveOccurred(), out)
				_, err = os.Stat(filepath.Join(tempDir, "kairos-rockylinux-9-core-amd64-generic-v3.2.1.raw.gce.tar.gz"))
				// Open the file and check that there is a disk.raw file inside and check that its rounded to a GB
				Expect(err).ToNot(HaveOccurred(), out)
			})
			It("generates a vhd image", func() {
				out, err := aurora.Run("--debug",
					"--set", "disable_http_server=true",
					"--set", "disable_netboot=true",
					"--set", "artifact_version=v3.2.1",
					"--set", "release_version=v3.2.1",
					"--set", "flavor=rockylinux",
					"--set", "flavor_release=9",
					"--set", "repository=kairos-io/kairos",
					"--set", "disable_netboot=true",
					"--set", "state_dir=/tmp/auroraboot",
					"--set", "disk.vhd=true",
					"--cloud-config", "/config.yaml")
				Expect(out).To(ContainSubstring("Generating raw disk"), out)
				Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
				Expect(out).To(ContainSubstring("gen-raw-disk"), out)
				Expect(out).To(ContainSubstring("download-squashfs"), out)
				Expect(out).To(ContainSubstring("extract-squashfs"), out)
				Expect(out).ToNot(ContainSubstring("dump-source"), out)
				Expect(err).ToNot(HaveOccurred(), out)
				_, err = os.Stat(filepath.Join(tempDir, "kairos-rockylinux-9-core-amd64-generic-v3.2.1.raw.vhd"))
				Expect(err).ToNot(HaveOccurred(), out)
			})
		})
	})

	Context("source is a container image", func() {
		var config string

		BeforeEach(func() {
			// Overwrite the config.yaml file with a cloud-config
			config = `#cloud-config

hostname: kairos-{{ trunc 4 .MachineID }}

# Automated install block
install:
  # Device for automated installs
  device: "auto"
  # Reboot after installation
  reboot: false
  # Power off after installation
  poweroff: true
  # Set to true to enable automated installations
  auto: true

## Login
users:
- name: "kairos"
  groups:
    - "admin"
  lock_passwd: true
  ssh_authorized_keys:
  - github:mudler

stages:
  boot:
  - name: "Repart image"
    layout:
      device:
        label: COS_PERSISTENT
      expand_partition:
        size: 0 # all space
    commands:
      # grow filesystem if not used 100%
      - |
         [[ "$(echo "$(df -h | grep COS_PERSISTENT)" | awk '{print $5}' | tr -d '%')" -ne 100 ]] && resize2fs /dev/disk/by-label/COS_PERSISTENT`
			err = WriteConfig(config, tempDir)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tempDir)
		})
		Describe("EFI", Label("efi"), func() {
			It("generate a raw disk file", func() {
				image := "quay.io/kairos/opensuse:tumbleweed-core-amd64-generic-v3.2.1"
				_, err := PullImage(image)
				Expect(err).ToNot(HaveOccurred())

				out, err := aurora.Run("--debug",
					"--set", "disable_http_server=true",
					"--set", "disable_netboot=true",
					"--set", "container_image=docker://"+image,
					"--set", "state_dir=/tmp/auroraboot",
					"--set", "disk.raw=true",
					"--cloud-config", "/config.yaml",
				)

				Expect(out).To(ContainSubstring("Generating raw disk"), out)
				Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
				Expect(out).To(ContainSubstring("gen-raw-disk"), out)
				Expect(out).To(ContainSubstring("dump-source"), out)
				Expect(err).ToNot(HaveOccurred(), out)
				_, err = os.Stat(filepath.Join(tempDir, "kairos-opensuse-tumbleweed-core-amd64-generic-v3.2.1.raw"))
				Expect(err).ToNot(HaveOccurred(), out)
			})
			It("generates a gce image", func() {
				image := "quay.io/kairos/opensuse:tumbleweed-core-amd64-generic-v3.2.1"
				_, err := PullImage(image)
				Expect(err).ToNot(HaveOccurred())

				out, err := aurora.Run("--debug",
					"--set", "disable_http_server=true",
					"--set", "disable_netboot=true",
					"--set", "container_image=docker://"+image,
					"--set", "state_dir=/tmp/auroraboot",
					"--set", "disk.gce=true",
					"--cloud-config", "/config.yaml",
				)
				Expect(out).To(ContainSubstring("Generating raw disk"), out)
				Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
				Expect(out).To(ContainSubstring("gen-raw-disk"), out)
				Expect(out).To(ContainSubstring("convert-gce"), out)
				Expect(out).To(ContainSubstring("dump-source"), out)
				Expect(err).ToNot(HaveOccurred(), out)
				_, err = os.Stat(filepath.Join(tempDir, "kairos-opensuse-tumbleweed-core-amd64-generic-v3.2.1.raw.gce.tar.gz"))
				Expect(err).ToNot(HaveOccurred(), out)
			})
			It("generates a vhd image", func() {
				image := "quay.io/kairos/opensuse:tumbleweed-core-amd64-generic-v3.2.1"
				_, err := PullImage(image)
				Expect(err).ToNot(HaveOccurred())

				out, err := aurora.Run("--debug",
					"--set", "disable_http_server=true",
					"--set", "disable_netboot=true",
					"--set", "container_image=docker://"+image,
					"--set", "state_dir=/tmp/auroraboot",
					"--set", "disk.vhd=true",
					"--cloud-config", "/config.yaml",
				)
				Expect(out).To(ContainSubstring("Generating raw disk"), out)
				Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
				Expect(out).To(ContainSubstring("gen-raw-disk"), out)
				Expect(out).To(ContainSubstring("convert-vhd"), out)
				Expect(out).To(ContainSubstring("dump-source"), out)
				Expect(err).ToNot(HaveOccurred(), out)
				_, err = os.Stat(filepath.Join(tempDir, "kairos-opensuse-tumbleweed-core-amd64-generic-v3.2.1.raw.vhd"))
				Expect(err).ToNot(HaveOccurred(), out)
			})
		})
		Describe("MBR", Label("mbr"), func() {
			It("generates a raw image", func() {
				image := "quay.io/kairos/opensuse:tumbleweed-core-amd64-generic-v3.2.1"
				_, err := PullImage(image)
				Expect(err).ToNot(HaveOccurred())

				out, err := aurora.Run("--debug",
					"--set", "disable_http_server=true",
					"--set", "disable_netboot=true",
					"--set", "container_image=docker://"+image,
					"--set", "state_dir=/tmp/auroraboot",
					"--set", "disk.mbr=true",
					"--cloud-config", "/config.yaml",
				)
				Expect(out).To(ContainSubstring("Generating MBR disk"), out)
				Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
				Expect(out).To(ContainSubstring("gen-raw-mbr-disk"), out)
				Expect(out).To(ContainSubstring("dump-source"), out)
				Expect(err).ToNot(HaveOccurred(), out)
				_, err = os.Stat(filepath.Join(tempDir, "disk.raw"))
				Expect(err).ToNot(HaveOccurred(), out)
			})
		})
	})
})
