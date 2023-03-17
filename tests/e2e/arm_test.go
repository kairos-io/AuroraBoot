package auroraboot_test

import (
	"fmt"
	"os"

	"github.com/kairos-io/kairos/pkg/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ARM image generation", Label("arm"), func() {
	Context("build", func() {
		It("generate a disk.img file", func() {
			testScript := `
			IMAGE=quay.io/kairos/core-opensuse-leap-arm-rpi:latest
			docker pull $IMAGE
			docker run --privileged -v $PWD/config.yaml:/config.yaml \
									-v $PWD/build:/tmp/auroraboot \
									-v /var/run/docker.sock:/var/run/docker.sock \
									-v $PWD/data:/tmp/data --rm auroraboot \
									--set "disable_http_server=true" \
									--set "disable_netboot=true" \
									--set "container_image=docker://$IMAGE" \
									--set "state_dir=/tmp/auroraboot" \
									--set "disk.arm.model=rpi64" \
									--cloud-config /config.yaml --debug
			`
			out, err := utils.SH(testScript)
			fmt.Println(out)
			Expect(out).To(ContainSubstring("done"), out)
			Expect(out).To(ContainSubstring("build-arm-image"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat("build/iso/disk.img")
			Expect(err).ToNot(HaveOccurred())
		})

		It("prepare partition files", func() {
			testScript := `
			IMAGE=quay.io/kairos/core-opensuse-leap-arm-rpi:latest
			docker pull $IMAGE
			docker run --privileged -v $PWD/config.yaml:/config.yaml \
									-v $PWD/build:/tmp/auroraboot \
									-v /var/run/docker.sock:/var/run/docker.sock \
									-v $PWD/data:/tmp/data --rm auroraboot \
									--set "disable_http_server=true" \
									--set "disable_netboot=true" \
									--set "container_image=docker://$IMAGE" \
									--set "state_dir=/tmp/auroraboot" \
									--set "disk.arm.prepare_only=true" \
									--cloud-config /config.yaml --debug
			`
			out, err := utils.SH(testScript)
			fmt.Println(out)
			Expect(out).To(ContainSubstring("done"), out)
			Expect(out).ToNot(ContainSubstring("build-arm-image"), out)
			Expect(out).To(ContainSubstring("prepare_arm"), out)
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat("build/iso/efi.img")
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
