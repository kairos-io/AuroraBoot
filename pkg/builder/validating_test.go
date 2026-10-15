package builder_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
)

type recordingArtifactBuilder struct {
	builds    []builder.BuildOptions
	cancelled []string
}

func (r *recordingArtifactBuilder) Build(_ context.Context, opts builder.BuildOptions) (*builder.BuildStatus, error) {
	r.builds = append(r.builds, opts)
	return &builder.BuildStatus{ID: opts.ID, Phase: builder.BuildPending}, nil
}

func (r *recordingArtifactBuilder) Status(_ context.Context, id string) (*builder.BuildStatus, error) {
	return &builder.BuildStatus{ID: id, Phase: builder.BuildReady}, nil
}

func (r *recordingArtifactBuilder) List(context.Context) ([]*builder.BuildStatus, error) {
	return []*builder.BuildStatus{{ID: "listed"}}, nil
}

func (r *recordingArtifactBuilder) Cancel(_ context.Context, id string) error {
	r.cancelled = append(r.cancelled, id)
	return nil
}

type recordingExtensionBuilder struct {
	builds    []builder.ExtensionBuildOptions
	cancelled []string
}

func (r *recordingExtensionBuilder) Build(_ context.Context, opts builder.ExtensionBuildOptions) (*builder.ExtensionBuildStatus, error) {
	r.builds = append(r.builds, opts)
	return &builder.ExtensionBuildStatus{ID: opts.ID, Phase: builder.BuildPending}, nil
}

func (r *recordingExtensionBuilder) Status(_ context.Context, id string) (*builder.ExtensionBuildStatus, error) {
	return &builder.ExtensionBuildStatus{ID: id, Phase: builder.BuildReady}, nil
}

func (r *recordingExtensionBuilder) List(context.Context) ([]*builder.ExtensionBuildStatus, error) {
	return []*builder.ExtensionBuildStatus{{ID: "listed"}}, nil
}

func (r *recordingExtensionBuilder) Cancel(_ context.Context, id string) error {
	r.cancelled = append(r.cancelled, id)
	return nil
}

var _ = Describe("ValidatingArtifactBuilder", func() {
	var (
		inner *recordingArtifactBuilder
		b     builder.ArtifactBuilder
		ctx   context.Context
	)

	BeforeEach(func() {
		inner = &recordingArtifactBuilder{}
		b = builder.NewValidatingArtifactBuilder(inner)
		ctx = context.Background()
	})

	DescribeTable("refuses a build whose image reference would change a generated Dockerfile",
		func(opts builder.BuildOptions, field string) {
			_, err := b.Build(ctx, opts)
			Expect(err).To(MatchError(builder.ErrInvalidBuildOptions))
			Expect(err.Error()).To(ContainSubstring(field))
			Expect(inner.builds).To(BeEmpty())
		},
		Entry("base image", builder.BuildOptions{BaseImage: "ubuntu:24.04\nRUN id"}, "base image"),
		Entry("source base image", builder.BuildOptions{Source: builder.ImageSource{BaseImage: "ubuntu:24.04\nRUN id"}}, "base image"),
		Entry("kairos-init image", builder.BuildOptions{KairosInitImage: "quay.io/kairos/kairos-init:v0.5.0\nRUN id"}, "kairos-init image"),
		Entry("hadron base", builder.BuildOptions{HadronBase: "quay.io/kairos/hadron:latest\nRUN id"}, "hadron base"),
		Entry("resolver URI", builder.BuildOptions{BaseImage: "dir:/var/lib/auroraboot"}, "base image"),
	)

	It("hands a build with valid references to the wrapped builder unchanged", func() {
		opts := builder.BuildOptions{
			ID:              "a-1",
			BaseImage:       "ubuntu:24.04",
			KairosInitImage: "quay.io/kairos/kairos-init:v0.5.0",
			HadronBase:      "quay.io/kairos/hadron:latest",
		}
		status, err := b.Build(ctx, opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(status.ID).To(Equal("a-1"))
		Expect(inner.builds).To(ConsistOf(opts))
	})

	It("passes Status, List and Cancel through", func() {
		status, err := b.Status(ctx, "a-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Phase).To(Equal(builder.BuildReady))

		list, err := b.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(list).To(HaveLen(1))

		Expect(b.Cancel(ctx, "a-1")).To(Succeed())
		Expect(inner.cancelled).To(ConsistOf("a-1"))
	})
})

var _ = Describe("ValidatingExtensionBuilder", func() {
	var (
		inner *recordingExtensionBuilder
		b     builder.ExtensionBuilder
		ctx   context.Context
	)

	BeforeEach(func() {
		inner = &recordingExtensionBuilder{}
		b = builder.NewValidatingExtensionBuilder(inner)
		ctx = context.Background()
	})

	It("refuses a build whose source image would change a generated Dockerfile", func() {
		_, err := b.Build(ctx, builder.ExtensionBuildOptions{
			Source: builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04\nRUN id"},
		})
		Expect(err).To(MatchError(builder.ErrInvalidBuildOptions))
		Expect(err.Error()).To(ContainSubstring("source.baseImage"))
		Expect(inner.builds).To(BeEmpty())
	})

	It("hands a build with a valid source image to the wrapped builder unchanged", func() {
		opts := builder.ExtensionBuildOptions{
			ID:     "e-1",
			Source: builder.ExtensionSource{Mode: "image", BaseImage: "quay.io/kairos/ubuntu:24.04"},
		}
		_, err := b.Build(ctx, opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(inner.builds).To(HaveLen(1))
		Expect(inner.builds[0].Source.BaseImage).To(Equal("quay.io/kairos/ubuntu:24.04"))
	})

	It("passes Status, List and Cancel through", func() {
		status, err := b.Status(ctx, "e-1")
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Phase).To(Equal(builder.BuildReady))

		list, err := b.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(list).To(HaveLen(1))

		Expect(b.Cancel(ctx, "e-1")).To(Succeed())
		Expect(inner.cancelled).To(ConsistOf("e-1"))
	})
})
