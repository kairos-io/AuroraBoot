package auroraboot_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ISO image generation", Label("iso"), func() {
	Context("build", func() {

		tempDir := ""
		BeforeEach(func() {
			t, err := os.MkdirTemp("", "")
			Expect(err).ToNot(HaveOccurred())

			tempDir = t

			err = WriteConfig("test", t)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(tempDir)
		})

		It("generate an iso image from a container", func() {
			image := "quay.io/kairos/core-rockylinux:latest"
			_, err := PullImage(image)
			Expect(err).ToNot(HaveOccurred())

			out, err := RunAurora(fmt.Sprintf(`--set container_image=docker://%s \
			--set "disable_http_server=true" \
			--set "disable_netboot=true" \
			--cloud-config /config.yaml \
			--set "state_dir=/tmp/auroraboot"`, image), tempDir)
			Expect(out).To(ContainSubstring("Generating iso"), out)
			Expect(out).To(ContainSubstring("gen-iso"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "build/build/kairos.iso"))
			Expect(err).ToNot(HaveOccurred())
		})
		It("fails if cloud config is empty", func() {
			err := WriteConfig("", tempDir)
			Expect(err).ToNot(HaveOccurred())

			out, err := RunAurora(fmt.Sprintf(`--set container_image=quay.io/kairos/core-rockylinux:latest \
			--set "disable_http_server=true" \
			--set "disable_netboot=true" \
			--cloud-config /config.yaml \
			--set "state_dir=/tmp/auroraboot"`), tempDir)
			Expect(err).To(HaveOccurred(), out)
			Expect(out).To(MatchRegexp("cloud config set but contents are empty"))
		})

		It("generate an iso image from a release", func() {
			out, err := RunAurora(`--set "disable_http_server=true" \
			--set "artifact_version=v2.4.2" \
			--set "release_version=v2.4.2" \
			--set "flavor=rockylinux" \
			--set "flavor_release=9" \
			--set repository="kairos-io/kairos" \
			--set "disable_netboot=true" \
			--cloud-config /config.yaml \
			--set "state_dir=/tmp/auroraboot"`, tempDir)
			Expect(out).To(ContainSubstring("Adding cloud config file"), out)
			Expect(out).ToNot(ContainSubstring("gen-iso"), out)
			Expect(out).To(ContainSubstring("download-iso"), out)
			Expect(out).To(ContainSubstring("inject-cloud-config"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(filepath.Join(tempDir, "build/build/kairos.iso.custom.iso"))
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
