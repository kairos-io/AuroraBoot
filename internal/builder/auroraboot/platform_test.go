package auroraboot

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// buildxInspectOutput is the shape of a real "docker buildx inspect" reply on a
// host that has qemu-user-static registered for the foreign architectures.
const buildxInspectOutput = `Name:          default
Driver:        docker-container
Last Activity: 2026-09-07 10:11:12 +0000 UTC

Nodes:
Name:      default0
Endpoint:  unix:///var/run/docker.sock
Status:    running
Platforms: linux/arm64*, linux/amd64, linux/arm/v7, linux/386
`

var _ = Describe("Build platform preflight", func() {
	var restore func()

	BeforeEach(func() {
		previous := buildxPlatforms
		restore = func() { buildxPlatforms = previous }
	})

	AfterEach(func() { restore() })

	stub := func(platforms []string, err error) {
		buildxPlatforms = func(context.Context) ([]string, error) { return platforms, err }
	}

	It("parses the platform list, dropping the preferred marker", func() {
		Expect(parseBuildxPlatforms(buildxInspectOutput)).To(Equal([]string{
			"linux/arm64", "linux/amd64", "linux/arm/v7", "linux/386",
		}))
	})

	It("reads every node's platforms", func() {
		out := "Platforms: linux/arm64*\nPlatforms: linux/amd64\n"
		Expect(parseBuildxPlatforms(out)).To(Equal([]string{"linux/arm64", "linux/amd64"}))
	})

	It("finds no platforms in unrelated output", func() {
		Expect(parseBuildxPlatforms("Name: default\nDriver: docker\n")).To(BeEmpty())
	})

	It("accepts an architecture the builder can emulate", func() {
		stub([]string{"linux/arm64", "linux/amd64"}, nil)
		Expect(checkBuildPlatform(context.Background(), "amd64")).To(Succeed())
	})

	It("accepts an architecture the builder offers only as a variant", func() {
		stub([]string{"linux/arm64", "linux/arm/v7"}, nil)
		Expect(checkBuildPlatform(context.Background(), "arm")).To(Succeed())
	})

	It("rejects an architecture the builder cannot run, and says how to fix it", func() {
		stub([]string{"linux/arm64"}, nil)
		err := checkBuildPlatform(context.Background(), "amd64")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(And(
			ContainSubstring("cannot build for linux/amd64"),
			ContainSubstring("it offers linux/arm64"),
			ContainSubstring("binfmt_misc"),
			ContainSubstring("tonistiigi/binfmt --install amd64"),
		))
	})

	It("skips the check when no architecture was requested", func() {
		stub(nil, errors.New("buildx must not be consulted"))
		Expect(checkBuildPlatform(context.Background(), "")).To(Succeed())
	})

	It("allows the build when buildx cannot be probed", func() {
		stub(nil, errors.New("docker buildx inspect: executable file not found"))
		Expect(checkBuildPlatform(context.Background(), "amd64")).To(Succeed())
	})

	It("allows the build when buildx reports no platforms at all", func() {
		stub(nil, nil)
		Expect(checkBuildPlatform(context.Background(), "amd64")).To(Succeed())
	})
})
