package auroraboot_test

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/internal/builder/auroraboot"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

func noopDeploy(_ context.Context, _ schema.Config, _ schema.ReleaseArtifact, _ string, _ io.Writer) error {
	return nil
}

// recStore is a minimal ArtifactStore that captures the record the builder
// persists at Build time so a test can assert every field the frontend later
// clones from is on the row. Guarded because b.run() spawns a goroutine that
// also touches the record via UpdatePhaseMessage.
type recStore struct {
	mu   sync.Mutex
	recs map[string]*store.ArtifactRecord
}

func newRecStore() *recStore { return &recStore{recs: map[string]*store.ArtifactRecord{}} }

func (s *recStore) Create(_ context.Context, rec *store.ArtifactRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *rec
	s.recs[rec.ID] = &cp
	return nil
}
func (s *recStore) GetByID(_ context.Context, id string) (*store.ArtifactRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	cp := *r
	return &cp, nil
}
func (s *recStore) List(_ context.Context) ([]*store.ArtifactRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*store.ArtifactRecord, 0, len(s.recs))
	for _, r := range s.recs {
		out = append(out, r)
	}
	return out, nil
}
func (s *recStore) Update(_ context.Context, rec *store.ArtifactRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.recs[rec.ID]; !ok {
		return fmt.Errorf("not found")
	}
	cp := *rec
	s.recs[rec.ID] = &cp
	return nil
}
func (s *recStore) UpdatePhaseMessage(_ context.Context, id, phase, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.recs[id]; ok {
		r.Phase = phase
		r.Message = message
	}
	return nil
}
func (s *recStore) UpdateFiles(_ context.Context, id string, files []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.recs[id]; ok {
		r.ArtifactFiles = files
	}
	return nil
}
func (s *recStore) ClearUploadToken(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.recs[id]; ok {
		r.UploadToken = ""
	}
	return nil
}
func (s *recStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.recs, id)
	return nil
}
func (s *recStore) DeleteByPhase(_ context.Context, _ string) error   { return nil }
func (s *recStore) GetLogs(_ context.Context, _ string) (string, error) { return "", nil }
func (s *recStore) AppendLog(_ context.Context, _ string, _ string) error {
	return nil
}

var _ = Describe("AuroraBoot Builder record persistence", func() {
	// This spec pins the fields the ArtifactBuilder's clone flow reads back
	// out of the artifact row. The handler's own store.Create at
	// pkg/handlers/artifacts.go is skipped when the builder already wrote
	// the row, so anything the builder omits here is silently dropped for
	// local builds and the cloned form comes up empty.
	It("persists the Kubernetes fields the clone flow reads from the row", func() {
		s := newRecStore()
		b := auroraboot.New(GinkgoT().TempDir(), noopDeploy, s)
		// The build keeps writing into the TempDir after Build returns, so
		// join it before Ginkgo removes that tree.
		DeferCleanup(func() { Expect(b.Shutdown(context.Background())).To(Succeed()) })

		_, err := b.Build(context.Background(), builder.BuildOptions{
			ID:            "clone-k8s",
			BaseImage:     "quay.io/kairos/ubuntu:latest",
			KairosVersion: "v4.1.2",
			Model:         "generic",
			Source: builder.ImageSource{
				BaseImage:         "quay.io/kairos/ubuntu:latest",
				KairosVersion:     "v4.1.2",
				Model:             "generic",
				Arch:              "amd64",
				Variant:           "standard",
				KubernetesDistro:  "k3s",
				KubernetesVersion: "v1.31.4+k3s1",
			},
			Provisioning: builder.ProvisioningOptions{
				KubernetesEnabled: true,
				TargetGroupID:     "grp-1",
			},
			OverlayRootfs: "/tmp/overlay",
		})
		Expect(err).NotTo(HaveOccurred())

		rec, err := s.GetByID(context.Background(), "clone-k8s")
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Variant).To(Equal("standard"))
		Expect(rec.KubernetesDistro).To(Equal("k3s"))
		Expect(rec.KubernetesVersion).To(Equal("v1.31.4+k3s1"))
		Expect(rec.KubernetesEnabled).NotTo(BeNil())
		Expect(*rec.KubernetesEnabled).To(BeTrue())
		Expect(rec.TargetGroupID).To(Equal("grp-1"))
		Expect(rec.OverlayRootfs).To(Equal("/tmp/overlay"))
	})
})

var _ = Describe("AuroraBoot Builder record persistence of catalog extensions", func() {
	// The artifact page's Build summary and the Clone flow read the catalog
	// extensions back from the row (kairos-io/kairos#5274). The handler's
	// own record is skipped when the builder already wrote one, so the
	// builder must persist them itself.
	It("persists Extensions and ExtensionsCatalogs", func() {
		s := newRecStore()
		b := auroraboot.New(GinkgoT().TempDir(), noopDeploy, s)

		_, err := b.Build(context.Background(), builder.BuildOptions{
			ID:                 "with-extensions",
			BaseImage:          "quay.io/kairos/hadron:latest",
			Extensions:         []string{"tailscale", "k3s@v1.31.4"},
			ExtensionsCatalogs: []string{"https://example.com/releases.json"},
		})
		Expect(err).NotTo(HaveOccurred())

		// Wait for the build goroutine to finish so the record is read after
		// the post-build phase and file writes, and so the goroutine is not
		// still writing into the TempDir when Ginkgo removes it.
		Eventually(func() string {
			r, err := s.GetByID(context.Background(), "with-extensions")
			if err != nil {
				return ""
			}
			return r.Phase
		}, 10*time.Second).Should(BeElementOf(store.ArtifactReady, store.ArtifactError))

		rec, err := s.GetByID(context.Background(), "with-extensions")
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Extensions).To(Equal([]string{"tailscale", "k3s@v1.31.4"}))
		Expect(rec.ExtensionsCatalogs).To(Equal([]string{"https://example.com/releases.json"}))
	})
})
