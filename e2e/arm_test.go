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

			aurora = NewAuroraboot("auroraboot", fmt.Sprintf("%s/config.yaml", tempDir))
		})

		AfterEach(func() {
			os.RemoveAll(tempDir)
		})

		It("generate a disk.img file", func() {
			image := "quay.io/kairos/core-opensuse-leap-arm-rpi:latest"
			_, err := PullImage(image)
			Expect(err).ToNot(HaveOccurred())

			out, err := aurora.Run(fmt.Sprintf(`--set container_image=docker://%s \
			--set "disable_http_server=true" \
			--set "disable_netboot=true" \
			--cloud-config /config.yaml \
			--set "disk.arm.model=rpi4" \
			--set "state_dir=/tmp/auroraboot"`, image), tempDir)
			Expect(out).To(ContainSubstring("done"), out)
			Expect(out).To(ContainSubstring("build-arm-image"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "build/disk.img"))
			Expect(err).ToNot(HaveOccurred())
		})

		It("prepare partition files", func() {

			image := "quay.io/kairos/core-opensuse-leap-arm-rpi:latest"
			_, err := PullImage(image)
			Expect(err).ToNot(HaveOccurred())

			out, err := aurora.Run(fmt.Sprintf(`--set container_image=docker://%s \
			--set "disable_http_server=true" \
			--set "disable_netboot=true" \
			--cloud-config /config.yaml \
			--set "disk.arm.prepare_only=true" \
			--set "state_dir=/tmp/auroraboot"`, image), tempDir)
			Expect(out).To(ContainSubstring("done"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(out).To(ContainSubstring("prepare_arm"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "build/efi.img"))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
