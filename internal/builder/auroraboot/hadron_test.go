package auroraboot_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/internal/builder/auroraboot"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/hadron"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
)

var _ = Describe("AuroraBoot Builder — Hadron kind", func() {
	var (
		b       *auroraboot.Builder
		baseDir string
	)

	BeforeEach(func() {
		baseDir = GinkgoT().TempDir()
		// deployFunc must never be called for hadron builds. Wire one that
		// blows the test up if it is invoked.
		b = auroraboot.New(baseDir, func(_ context.Context, _ schema.Config, _ schema.ReleaseArtifact, _ string) error {
			Fail("deployFunc invoked for a hadron build")
			return nil
		}, nil)
	})

	It("routes a valid hadron spec to the hadron path and marks it Ready", func() {
		called := make(chan hadron.Spec, 1)
		b.WithHadronBuildFunc(func(_ context.Context, spec hadron.Spec, workDir string, _ hadron.RegistryAuthProvider, _ io.Writer) (*hadron.Result, error) {
			called <- spec
			return &hadron.Result{
				ImageRef:       spec.OutputRef,
				DockerfilePath: filepath.Join(workDir, "Dockerfile.hadron"),
				TarballPath:    filepath.Join(workDir, "hadron.oci.tar"),
			}, nil
		})

		spec := hadron.Spec{
			BaseImage:      "ghcr.io/kairos-io/hadron:main",
			Layers:         []string{"ghcr.io/kairos-io/git:latest"},
			Platforms:      []string{"linux/amd64"},
			OutputRef:      "example.com/team/os:v1",
			ProduceTarball: true,
		}
		status, err := b.Build(context.Background(), builder.BuildOptions{
			ID:     "hadron-1",
			Kind:   "hadron",
			Hadron: spec,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(status.Phase).To(Equal(builder.BuildPending))

		var got hadron.Spec
		Eventually(called, 5*time.Second).Should(Receive(&got))
		Expect(got).To(Equal(spec))

		// Give the goroutine a moment to flip Ready.
		Eventually(func() string {
			s, err := b.Status(context.Background(), "hadron-1")
			if err != nil {
				return ""
			}
			return s.Phase
		}, 5*time.Second, 50*time.Millisecond).Should(Equal(builder.BuildReady))
	})

	It("rejects an invalid hadron spec at Build() time (before starting a build)", func() {
		_, err := b.Build(context.Background(), builder.BuildOptions{
			ID:     "hadron-bad",
			Kind:   "hadron",
			Hadron: hadron.Spec{}, // empty → invalid
		})
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, builder.ErrInvalidBuildOptions)).To(BeTrue())
	})

	It("marks the build Error when the hadron pipeline fails", func() {
		b.WithHadronBuildFunc(func(_ context.Context, _ hadron.Spec, _ string, _ hadron.RegistryAuthProvider, _ io.Writer) (*hadron.Result, error) {
			return nil, errors.New("buildx exploded")
		})
		_, err := b.Build(context.Background(), builder.BuildOptions{
			ID:   "hadron-fail",
			Kind: "hadron",
			Hadron: hadron.Spec{
				BaseImage:      "ghcr.io/kairos-io/hadron:main",
				OutputRef:      "example.com/team/os:v1",
				ProduceTarball: true,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() string {
			s, err := b.Status(context.Background(), "hadron-fail")
			if err != nil {
				return ""
			}
			return s.Phase
		}, 5*time.Second, 50*time.Millisecond).Should(Equal(builder.BuildError))
	})

	It("propagates Cancel into the running hadron build ctx and marks the artifact Error", func() {
		started := make(chan struct{}, 1)
		b.WithHadronBuildFunc(func(ctx context.Context, _ hadron.Spec, _ string, _ hadron.RegistryAuthProvider, _ io.Writer) (*hadron.Result, error) {
			started <- struct{}{}
			// Block until the builder's Cancel propagates ctx cancellation.
			// If Cancel ever stops threading its cancelFunc through to the
			// hadron ctx, this select never returns and the surrounding
			// Eventually times out — that's the regression signal.
			<-ctx.Done()
			return nil, ctx.Err()
		})

		status, err := b.Build(context.Background(), builder.BuildOptions{
			ID:   "hadron-cancel",
			Kind: "hadron",
			Hadron: hadron.Spec{
				BaseImage:      "ghcr.io/kairos-io/hadron:main",
				OutputRef:      "example.com/team/os:v1",
				Platforms:      []string{"linux/amd64"},
				ProduceTarball: true,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		Eventually(started, 2*time.Second).Should(Receive())

		Expect(b.Cancel(context.Background(), status.ID)).To(Succeed())

		Eventually(func(g Gomega) {
			s, sErr := b.Status(context.Background(), status.ID)
			g.Expect(sErr).NotTo(HaveOccurred())
			g.Expect(s.Phase).To(Equal(builder.BuildError))
			// Cancel writes "cancelled"; the runHadron error path may race
			// in with "hadron build failed: context canceled". Either
			// spelling satisfies the "reached Error via cancellation" claim.
			g.Expect(s.Message).To(MatchRegexp(`(?i)cancel`))
		}, 2*time.Second, 20*time.Millisecond).Should(Succeed())
	})
})
