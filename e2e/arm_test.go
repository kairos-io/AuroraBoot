package e2e_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ARM image generation", Label("arm"), func() {
	Context("build", func() {
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

		It("generate a disk.img file", func() {
			image := "quay.io/kairos/core-opensuse-leap-arm-rpi:latest"
			_, err := PullImage(image)
			Expect(err).ToNot(HaveOccurred())

			out, err := aurora.Run("--debug",
				"--set", "disable_http_server=true",
				"--set", "disable_netboot=true",
				"--set", "container_image=docker://"+image,
				"--set", "state_dir=/tmp/auroraboot",
				"--set", "disk.arm.model=rpi4",
				"--cloud-config", "/config.yaml",
			)
			Expect(out).To(ContainSubstring("done"), out)
			Expect(out).To(ContainSubstring("build-arm-image"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "disk.img"))
			Expect(err).ToNot(HaveOccurred())
		})

		It("prepare partition files", func() {

			image := "quay.io/kairos/core-opensuse-leap-arm-rpi:latest"
			_, err := PullImage(image)
			Expect(err).ToNot(HaveOccurred())

			out, err := aurora.Run("--debug",
				"--set", "disable_http_server=true",
				"--set", "disable_netboot=true",
				"--set", "container_image=docker://"+image,
				"--set", "state_dir=/tmp/auroraboot",
				"--set", "disk.arm.prepare_only=true",
				"--cloud-config", "/config.yaml",
			)
			Expect(out).To(ContainSubstring("done"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(out).To(ContainSubstring("prepare_arm"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "efi.img"))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
