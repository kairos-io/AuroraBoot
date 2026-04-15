package auroraboot_test

import (
	"context"
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/AuroraBoot/pkg/uki"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/internal/builder/auroraboot"
)

func assertError(msg string) error { return errors.New(msg) }

var _ = Describe("AuroraBoot Builder", func() {
	var (
		b       *auroraboot.Builder
		baseDir string
		// capturedConfig and capturedArtifact let tests inspect what was passed to the deployer.
		capturedConfig   schema.Config
		capturedArtifact schema.ReleaseArtifact
		mu               sync.Mutex
		deployCalled     chan struct{}
	)

	BeforeEach(func() {
		baseDir = GinkgoT().TempDir()
		ch := make(chan struct{}, 1)
		deployCalled = ch

		mockDeploy := func(_ context.Context, config schema.Config, artifact schema.ReleaseArtifact, _ string) error {
			mu.Lock()
			capturedConfig = config
			capturedArtifact = artifact
			mu.Unlock()
			ch <- struct{}{} // use local var to avoid race with BeforeEach reassignment
			return nil
		}

		b = auroraboot.New(baseDir, mockDeploy, nil)
	})

	// waitForBuild waits until the mock deployer has been invoked.
	waitForBuild := func() {
		Eventually(deployCalled, 5*time.Second).Should(Receive())
		// Give a tiny bit of time for status to update after deployFunc returns.
		time.Sleep(50 * time.Millisecond)
	}

	Describe("Build", func() {
		It("should create a build entry with Pending status", func() {
			status, err := b.Build(context.Background(), builder.BuildOptions{
				ID:        "test-1",
				BaseImage: "quay.io/kairos/ubuntu:latest",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(status.ID).To(Equal("test-1"))
			Expect(status.Phase).To(Equal(builder.BuildPending))
		})

		It("should transition to Building status", func() {
			// Use a deployer that blocks until we release it.
			blocked := make(chan struct{})
			slowDeploy := func(ctx context.Context, _ schema.Config, _ schema.ReleaseArtifact, _ string) error {
				select {
				case <-blocked:
				case <-ctx.Done():
				}
				return nil
			}
			slowBuilder := auroraboot.New(baseDir, slowDeploy, nil)

			_, err := slowBuilder.Build(context.Background(), builder.BuildOptions{
				ID:        "test-building",
				BaseImage: "quay.io/kairos/ubuntu:latest",
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait a moment for the goroutine to start and transition to Building.
			Eventually(func() string {
				s, _ := slowBuilder.Status(context.Background(), "test-building")
				if s == nil {
					return ""
				}
				return s.Phase
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(builder.BuildBuilding))

			close(blocked)
		})

		It("should set OverlayRootfs in AuroraBoot config when provided", func() {
			_, err := b.Build(context.Background(), builder.BuildOptions{
				ID:            "test-overlay",
				BaseImage:     "quay.io/kairos/ubuntu:latest",
				OverlayRootfs: "/tmp/overlay",
			})
			Expect(err).NotTo(HaveOccurred())
			waitForBuild()

			mu.Lock()
			defer mu.Unlock()
			Expect(capturedConfig.ISO.OverlayRootfs).To(Equal("/tmp/overlay"))
		})

		It("should inject cloud-config into AuroraBoot config", func() {
			cc := "#cloud-config\nusers:\n- name: test\n"
			_, err := b.Build(context.Background(), builder.BuildOptions{
				ID:          "test-cc",
				BaseImage:   "quay.io/kairos/ubuntu:latest",
				CloudConfig: cc,
			})
			Expect(err).NotTo(HaveOccurred())
			waitForBuild()

			mu.Lock()
			defer mu.Unlock()
			Expect(capturedConfig.CloudConfig).To(Equal(cc))
		})

		It("should set ContainerImage from BaseImage", func() {
			_, err := b.Build(context.Background(), builder.BuildOptions{
				ID:        "test-img",
				BaseImage: "quay.io/kairos/ubuntu:24.04-core-amd64-generic-v3.6.0",
			})
			Expect(err).NotTo(HaveOccurred())
			waitForBuild()

			mu.Lock()
			defer mu.Unlock()
			Expect(capturedArtifact.ContainerImage).To(Equal("quay.io/kairos/ubuntu:24.04-core-amd64-generic-v3.6.0"))
		})

		It("should reach Ready status after successful build", func() {
			_, err := b.Build(context.Background(), builder.BuildOptions{
				ID:        "test-ready",
				BaseImage: "quay.io/kairos/ubuntu:latest",
			})
			Expect(err).NotTo(HaveOccurred())
			waitForBuild()

			status, err := b.Status(context.Background(), "test-ready")
			Expect(err).NotTo(HaveOccurred())
			Expect(status.Phase).To(Equal(builder.BuildReady))
		})
	})

	Describe("Status", func() {
		It("should return status for known build ID", func() {
			_, err := b.Build(context.Background(), builder.BuildOptions{
				ID:        "known-id",
				BaseImage: "quay.io/kairos/ubuntu:latest",
			})
			Expect(err).NotTo(HaveOccurred())

			status, err := b.Status(context.Background(), "known-id")
			Expect(err).NotTo(HaveOccurred())
			Expect(status.ID).To(Equal("known-id"))
		})

		It("should return error for unknown ID", func() {
			_, err := b.Status(context.Background(), "does-not-exist")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("List", func() {
		It("should return all builds", func() {
			_, err := b.Build(context.Background(), builder.BuildOptions{
				ID:        "list-1",
				BaseImage: "img1",
			})
			Expect(err).NotTo(HaveOccurred())
			waitForBuild()

			// Reset channel for second build.
			deployCalled = make(chan struct{}, 1)
			// Need a new builder to use the new channel — but we can just add a second build.
			_, err = b.Build(context.Background(), builder.BuildOptions{
				ID:        "list-2",
				BaseImage: "img2",
			})
			Expect(err).NotTo(HaveOccurred())

			list, err := b.List(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(len(list)).To(BeNumerically(">=", 2))

			ids := make([]string, len(list))
			for i, s := range list {
				ids[i] = s.ID
			}
			Expect(ids).To(ContainElements("list-1", "list-2"))
		})
	})

	Describe("Cancel", func() {
		It("should cancel a running build", func() {
			blocked := make(chan struct{})
			slowDeploy := func(ctx context.Context, _ schema.Config, _ schema.ReleaseArtifact, _ string) error {
				select {
				case <-blocked:
				case <-ctx.Done():
					return ctx.Err()
				}
				return nil
			}
			cancelBuilder := auroraboot.New(baseDir, slowDeploy, nil)

			_, err := cancelBuilder.Build(context.Background(), builder.BuildOptions{
				ID:        "cancel-me",
				BaseImage: "quay.io/kairos/ubuntu:latest",
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait for it to reach Building.
			Eventually(func() string {
				s, _ := cancelBuilder.Status(context.Background(), "cancel-me")
				if s == nil {
					return ""
				}
				return s.Phase
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(builder.BuildBuilding))

			err = cancelBuilder.Cancel(context.Background(), "cancel-me")
			Expect(err).NotTo(HaveOccurred())

			status, err := cancelBuilder.Status(context.Background(), "cancel-me")
			Expect(err).NotTo(HaveOccurred())
			Expect(status.Phase).To(Equal(builder.BuildError))
			Expect(status.Message).To(ContainSubstring("cancel"))
		})

		It("should return error for unknown build ID", func() {
			err := b.Cancel(context.Background(), "no-such-build")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("buildUKI wiring", func() {
		// These tests swap pkg/uki.Build for a capture function so we can
		// assert the options we build flow through unchanged — we're testing
		// the option mapping, not the actual UKI pipeline (AuroraBoot owns
		// that).
		var (
			ukiMu       sync.Mutex
			capturedUKI uki.Options
			ukiCalled   chan struct{}
		)

		BeforeEach(func() {
			ukiCalled = make(chan struct{}, 1)
			capture := func(o uki.Options) error {
				ukiMu.Lock()
				capturedUKI = o
				ukiMu.Unlock()
				ukiCalled <- struct{}{}
				return nil
			}
			// Override the default uki.Build with our capture function.
			b.WithUKIBuildFunc(capture)
		})

		It("passes SecureBoot keys, enroll mode, overlay and source through", func() {
			_, err := b.Build(context.Background(), builder.BuildOptions{
				ID:            "uki-ok",
				BaseImage:     "quay.io/kairos/ubuntu:latest",
				OverlayRootfs: "/tmp/my-overlay",
				Outputs: builder.OutputOptions{
					ISO: true, // so the deployer step runs and we hit buildUKI after
					UKI: true,
				},
				Signing: builder.SigningOptions{
					UKISecureBootKey:    "/keys/db.key",
					UKISecureBootCert:   "/keys/db.pem",
					UKITPMPCRKey:        "/keys/tpm.pem",
					UKIPublicKeysDir:    "/keys",
					UKISecureBootEnroll: "force",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// Wait for both the deployer step and the uki capture.
			Eventually(deployCalled, 5*time.Second).Should(Receive())
			Eventually(ukiCalled, 5*time.Second).Should(Receive())

			ukiMu.Lock()
			defer ukiMu.Unlock()
			Expect(capturedUKI.Source).To(Equal("docker:quay.io/kairos/ubuntu:latest"))
			Expect(capturedUKI.OutputDir).To(ContainSubstring("uki-ok"))
			Expect(capturedUKI.OutputType).To(Equal("iso"))
			Expect(capturedUKI.Name).To(Equal("kairos"))
			Expect(capturedUKI.SBKey).To(Equal("/keys/db.key"))
			Expect(capturedUKI.SBCert).To(Equal("/keys/db.pem"))
			Expect(capturedUKI.TPMPCRPrivateKey).To(Equal("/keys/tpm.pem"))
			Expect(capturedUKI.PublicKeysDir).To(Equal("/keys"))
			Expect(capturedUKI.SecureBootEnroll).To(Equal("force"))
			Expect(capturedUKI.OverlayRootfs).To(Equal("/tmp/my-overlay"))
		})

		It("fails the build with a helpful message when UKI keys are missing", func() {
			_, err := b.Build(context.Background(), builder.BuildOptions{
				ID:        "uki-nokeys",
				BaseImage: "quay.io/kairos/ubuntu:latest",
				Outputs: builder.OutputOptions{
					ISO: true,
					UKI: true,
				},
				// No Signing populated.
			})
			Expect(err).NotTo(HaveOccurred())

			// Deployer still runs (iso), but uki.Build should never be called
			// because the pre-check rejects the empty keys first.
			Eventually(deployCalled, 5*time.Second).Should(Receive())
			Consistently(ukiCalled, 200*time.Millisecond).ShouldNot(Receive())

			// The build transitions to Error with a message mentioning the missing keys.
			Eventually(func() string {
				s, _ := b.Status(context.Background(), "uki-nokeys")
				if s == nil {
					return ""
				}
				return s.Phase
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(builder.BuildError))

			status, err := b.Status(context.Background(), "uki-nokeys")
			Expect(err).NotTo(HaveOccurred())
			Expect(status.Message).To(ContainSubstring("SecureBoot keys"))
		})

		It("surfaces pkg/uki.Build errors as a build failure", func() {
			// Override capture with a failing func.
			b.WithUKIBuildFunc(func(_ uki.Options) error {
				return assertError("synthetic uki failure")
			})

			_, err := b.Build(context.Background(), builder.BuildOptions{
				ID:        "uki-boom",
				BaseImage: "quay.io/kairos/ubuntu:latest",
				Outputs: builder.OutputOptions{
					ISO: true,
					UKI: true,
				},
				Signing: builder.SigningOptions{
					UKISecureBootKey:  "/keys/db.key",
					UKISecureBootCert: "/keys/db.pem",
					UKITPMPCRKey:      "/keys/tpm.pem",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Eventually(deployCalled, 5*time.Second).Should(Receive())

			Eventually(func() string {
				s, _ := b.Status(context.Background(), "uki-boom")
				if s == nil {
					return ""
				}
				return s.Phase
			}, 2*time.Second, 10*time.Millisecond).Should(Equal(builder.BuildError))

			status, err := b.Status(context.Background(), "uki-boom")
			Expect(err).NotTo(HaveOccurred())
			Expect(status.Message).To(ContainSubstring("synthetic uki failure"))
		})
	})
})
