package auroraboot

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
)

var _ = Describe("KairosInitImageFromEnv", func() {
	It("names the pinned default when KAIROS_INIT_IMAGE is unset", func() {
		GinkgoT().Setenv("KAIROS_INIT_IMAGE", "")
		Expect(os.Unsetenv("KAIROS_INIT_IMAGE")).To(Succeed())

		ref, err := KairosInitImageFromEnv()
		Expect(err).NotTo(HaveOccurred())
		Expect(ref).To(Equal(defaultKairosInitImage + ":" + defaultKairosInitVersion))
	})

	It("returns the configured reference", func() {
		GinkgoT().Setenv("KAIROS_INIT_IMAGE", "localhost:5000/kairos/kairos-init:dev")

		ref, err := KairosInitImageFromEnv()
		Expect(err).NotTo(HaveOccurred())
		Expect(ref).To(Equal("localhost:5000/kairos/kairos-init:dev"))
	})

	It("refuses a reference that would add lines to the kairosify Dockerfile", func() {
		GinkgoT().Setenv("KAIROS_INIT_IMAGE", "quay.io/kairos/kairos-init:v0.5.0\nRUN id")

		_, err := KairosInitImageFromEnv()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("KAIROS_INIT_IMAGE"))
	})
})

var _ = Describe("kairosify kairos-init image", func() {
	var outputDir string

	// A cancelled context stops the docker build from starting, which leaves
	// the rendered Dockerfile on disk for inspection.
	render := func(b *Builder, opts builder.BuildOptions) string {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := b.kairosify(ctx, "ubuntu:24.04", opts, outputDir, nil)
		Expect(err).To(HaveOccurred())
		data, err := os.ReadFile(filepath.Join(outputDir, "Dockerfile.kairosify"))
		Expect(err).NotTo(HaveOccurred())
		return string(data)
	}

	BeforeEach(func() {
		outputDir = GinkgoT().TempDir()
	})

	It("uses the pinned default when nothing else names one", func() {
		b := New(outputDir, nil, &kairosifyTestStore{})
		Expect(render(b, builder.BuildOptions{ID: "a-1"})).To(HavePrefix(
			"FROM " + defaultKairosInitImage + ":" + defaultKairosInitVersion + " AS kairos-init\n"))
	})

	It("uses the image the builder was configured with", func() {
		b := New(outputDir, nil, &kairosifyTestStore{}).WithKairosInitImage("localhost:5000/kairos/kairos-init:dev")
		Expect(render(b, builder.BuildOptions{ID: "a-1"})).To(HavePrefix(
			"FROM localhost:5000/kairos/kairos-init:dev AS kairos-init\n"))
	})

	It("prefers the image the build request names", func() {
		b := New(outputDir, nil, &kairosifyTestStore{}).WithKairosInitImage("localhost:5000/kairos/kairos-init:dev")
		Expect(render(b, builder.BuildOptions{ID: "a-1", KairosInitImage: "quay.io/kairos/kairos-init:v0.5.0"})).To(HavePrefix(
			"FROM quay.io/kairos/kairos-init:v0.5.0 AS kairos-init\n"))
	})
})
