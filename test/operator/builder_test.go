//go:build operator_e2e

package operator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	buildv1alpha2 "github.com/kairos-io/kairos-operator/api/v1alpha2"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"

	opbuilder "github.com/kairos-io/AuroraBoot/internal/builder/operator"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
)

// testLogSink implements builder.LogBroadcaster by collecting every chunk
// into a mutex-guarded slice so specs can Eventually-assert log arrival.
type testLogSink struct {
	mu     sync.Mutex
	chunks []string
}

func (s *testLogSink) BroadcastLogChunk(_ string, chunk string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks = append(s.chunks, chunk)
}

func (s *testLogSink) Snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.chunks))
	copy(out, s.chunks)
	return out
}

var _ = Describe("Operator builder against a real cluster", func() {
	const buildID = "auroraboot-builder-e2e"

	It("Build creates a CR that progresses toward Building; List reports it; Cancel removes it", func() {
		ctx := context.Background()

		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules, &clientcmd.ConfigOverrides{},
		).ClientConfig()
		Expect(err).NotTo(HaveOccurred(), "load REST config from KUBECONFIG")

		b, err := opbuilder.New(opbuilder.Config{
			RESTConfig: cfg,
			Namespace:  testNamespace,
		})
		Expect(err).NotTo(HaveOccurred(), "construct operator.Builder")

		status, err := b.Build(ctx, builder.BuildOptions{
			ID:        buildID,
			BaseImage: "quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.6.0",
			Source:    builder.ImageSource{Arch: "amd64"},
			Outputs:   builder.OutputOptions{ISO: true},
		})
		Expect(err).NotTo(HaveOccurred(), "Build should submit the CR")
		Expect(status.ID).To(Equal(buildID))
		Expect(status.Phase).To(Equal(builder.BuildPending))

		DeferCleanup(cleanupArtifact, ctx, buildID)
		DeferCleanup(func() {
			if CurrentSpecReport().Failed() {
				collectDebugLogs(ctx, buildID)
			}
		})

		got := &buildv1alpha2.OSArtifact{}
		Expect(testClient.Get(ctx, types.NamespacedName{Name: buildID, Namespace: testNamespace}, got)).To(Succeed())
		Expect(got.Spec.Image.Ref).To(Equal("quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.6.0"))

		Eventually(func() (string, error) {
			s, err := b.Status(ctx, buildID)
			if err != nil {
				return "", err
			}
			return s.Phase, nil
		}, 2*time.Minute, 2*time.Second).Should(Equal(builder.BuildBuilding), "Status reports Building")

		builds, err := b.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		ids := make([]string, 0, len(builds))
		for _, s := range builds {
			ids = append(ids, s.ID)
		}
		Expect(ids).To(ContainElement(buildID))

		Expect(b.Cancel(ctx, buildID)).To(Succeed())

		Eventually(func() bool {
			_, err := b.Status(ctx, buildID)
			return err != nil && errors.Is(err, opbuilder.ErrNotFound)
		}, 30*time.Second, 2*time.Second).Should(BeTrue(), "Status reports NotFound after Cancel")
	})

	It("streams build Pod logs to the attached LogBroadcaster", func() {
		const logsBuildID = "auroraboot-builder-e2e-logs"
		ctx := context.Background()

		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			loadingRules, &clientcmd.ConfigOverrides{},
		).ClientConfig()
		Expect(err).NotTo(HaveOccurred(), "load REST config from KUBECONFIG")

		sink := &testLogSink{}
		b, err := opbuilder.New(opbuilder.Config{
			RESTConfig: cfg,
			Namespace:  testNamespace,
		})
		Expect(err).NotTo(HaveOccurred(), "construct operator.Builder")
		b = b.WithLogBroadcaster(sink)

		status, err := b.Build(ctx, builder.BuildOptions{
			ID:        logsBuildID,
			BaseImage: "quay.io/kairos/opensuse:leap-15.6-core-amd64-generic-v3.6.0",
			Source:    builder.ImageSource{Arch: "amd64"},
			Outputs:   builder.OutputOptions{ISO: true},
		})
		Expect(err).NotTo(HaveOccurred(), "Build should submit the CR")
		Expect(status.ID).To(Equal(logsBuildID))

		DeferCleanup(cleanupArtifact, ctx, logsBuildID)
		DeferCleanup(func() {
			if CurrentSpecReport().Failed() {
				collectDebugLogs(ctx, logsBuildID)
			}
		})

		// The 3-minute budget covers a cold-cache first run in CI: kubelet
		// must pull the operator tool image (quay.io/kairos/auroraboot)
		// before the first init container can produce any output. Warm
		// runs finish in well under a minute.
		Eventually(func() bool {
			for _, chunk := range sink.Snapshot() {
				if strings.Contains(chunk, "auroraboot") ||
					strings.Contains(chunk, "buildah") ||
					strings.Contains(chunk, "kairos-release") ||
					strings.Contains(chunk, logsBuildID) {
					return true
				}
			}
			return false
		}, 3*time.Minute, 2*time.Second).Should(BeTrue(),
			"expected sink to collect at least one recognisable log line within 3m")
	})
})
