package auroraboot_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kairos-io/AuroraBoot/internal/builder/auroraboot"
	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// --- fake stores --------------------------------------------------------

// fakeExtStore is a thread-safe in-memory ExtensionStore for builder specs.
type fakeExtStore struct {
	mu   sync.Mutex
	rows map[string]*store.ExtensionRecord
}

func newFakeExtStore() *fakeExtStore {
	return &fakeExtStore{rows: map[string]*store.ExtensionRecord{}}
}

func (f *fakeExtStore) Create(_ context.Context, r *store.ExtensionRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *r
	f.rows[r.ID] = &cp
	return nil
}

func (f *fakeExtStore) GetByID(_ context.Context, id string) (*store.ExtensionRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; ok {
		cp := *r
		return &cp, nil
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (f *fakeExtStore) List(context.Context) ([]store.ExtensionRecord, error) { return nil, nil }
func (f *fakeExtStore) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, id)
	return nil
}
func (f *fakeExtStore) FindLatestReadyByName(context.Context, string, string) (*store.ExtensionRecord, error) {
	return nil, nil
}
func (f *fakeExtStore) FindByNameAndVersion(context.Context, string, string, string) (*store.ExtensionRecord, error) {
	return nil, nil
}
func (f *fakeExtStore) AppendLog(_ context.Context, id, chunk string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.rows[id]; ok {
		r.Logs += chunk
	}
	return nil
}

// Compile-time interface conformance check.
var _ store.ExtensionStore = (*fakeExtStore)(nil)

// fakeArtifactStore is just enough to resolve `artifact:<id>` source mode
// in the builder spec.
type fakeArtifactStore struct {
	rows map[string]*store.ArtifactRecord
}

func (f *fakeArtifactStore) Create(context.Context, *store.ArtifactRecord) error {
	return fmt.Errorf("unused")
}
func (f *fakeArtifactStore) GetByID(_ context.Context, id string) (*store.ArtifactRecord, error) {
	if r, ok := f.rows[id]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeArtifactStore) List(context.Context) ([]*store.ArtifactRecord, error) {
	return nil, fmt.Errorf("unused")
}
func (f *fakeArtifactStore) Update(context.Context, *store.ArtifactRecord) error {
	return fmt.Errorf("unused")
}
func (f *fakeArtifactStore) Delete(context.Context, string) error { return fmt.Errorf("unused") }
func (f *fakeArtifactStore) DeleteByPhase(context.Context, string) error {
	return fmt.Errorf("unused")
}
func (f *fakeArtifactStore) GetLogs(context.Context, string) (string, error) {
	return "", fmt.Errorf("unused")
}
func (f *fakeArtifactStore) AppendLog(context.Context, string, string) error {
	return fmt.Errorf("unused")
}

var _ store.ArtifactStore = (*fakeArtifactStore)(nil)

// --- specs --------------------------------------------------------------

var _ = Describe("ExtensionBuilder.Build (skeleton)", func() {
	var (
		extStore *fakeExtStore
		eb       *auroraboot.ExtensionBuilder
		ctx      = context.Background()
	)

	BeforeEach(func() {
		extStore = newFakeExtStore()
		eb = auroraboot.NewExtensionBuilder(GinkgoT().TempDir(), extStore).
			WithDockerBuildFunc(func(context.Context, auroraboot.DockerBuildArgs) error { return nil }).
			WithAurorabootCLIFunc(func(_ context.Context, a auroraboot.AurorabootCLIArgs) error {
				// Drop a fake .raw so updateRawFilename works.
				return os.WriteFile(filepath.Join(a.OutputDir, a.Name+"."+a.Type+".raw"), []byte("fake"), 0o644)
			})
	})

	It("persists a Pending record and returns immediately", func() {
		st, err := eb.Build(ctx, builder.ExtensionBuildOptions{
			ID: "e-1", Name: "tailscale-agent", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(st.ID).To(Equal("e-1"))
		Expect(st.Phase).To(Equal(builder.BuildPending))

		rec, gerr := extStore.GetByID(ctx, "e-1")
		Expect(gerr).ToNot(HaveOccurred())
		Expect(rec.Phase).To(BeElementOf(builder.BuildPending, builder.BuildBuilding, builder.BuildReady))
	})

	It("generates a UUID when ID is empty", func() {
		st, err := eb.Build(ctx, builder.ExtensionBuildOptions{
			Name: "x", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(st.ID).ToNot(BeEmpty())
	})
})

var _ = Describe("ExtensionBuilder.Build — source resolution", func() {
	var (
		eb       *auroraboot.ExtensionBuilder
		extStore *fakeExtStore
		artStore *fakeArtifactStore
		dbCalls  atomic.Int32
		dbArgs   auroraboot.DockerBuildArgs
		cliArgs  auroraboot.AurorabootCLIArgs
		argsMu   sync.Mutex
		baseDir  string
	)

	BeforeEach(func() {
		baseDir = GinkgoT().TempDir()
		extStore = newFakeExtStore()
		artStore = &fakeArtifactStore{rows: map[string]*store.ArtifactRecord{
			"a-1": {ID: "a-1", ContainerImage: "quay.io/myorg/edge-os:v4.1.0"},
		}}
		dbCalls.Store(0)
		eb = auroraboot.NewExtensionBuilder(baseDir, extStore).
			WithArtifactStore(artStore).
			WithDockerBuildFunc(func(_ context.Context, a auroraboot.DockerBuildArgs) error {
				dbCalls.Add(1)
				argsMu.Lock()
				dbArgs = a
				argsMu.Unlock()
				return nil
			}).
			WithAurorabootCLIFunc(func(_ context.Context, a auroraboot.AurorabootCLIArgs) error {
				argsMu.Lock()
				cliArgs = a
				argsMu.Unlock()
				return os.WriteFile(filepath.Join(a.OutputDir, a.Name+"."+a.Type+".raw"), []byte("fake"), 0o644)
			})
	})

	awaitReady := func(id string) *store.ExtensionRecord {
		Eventually(func() string {
			rec, _ := extStore.GetByID(context.Background(), id)
			if rec == nil {
				return ""
			}
			return rec.Phase
		}, "2s", "20ms").Should(Equal(builder.BuildReady))
		rec, _ := extStore.GetByID(context.Background(), id)
		return rec
	}

	It("uses BaseImage verbatim for Mode=image", func() {
		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-1", Name: "ts", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
		})
		Expect(err).ToNot(HaveOccurred())
		rec := awaitReady("e-1")
		Expect(rec.ContainerImage).To(Equal("ubuntu:24.04"))
		Expect(dbCalls.Load()).To(Equal(int32(0)))
		argsMu.Lock()
		defer argsMu.Unlock()
		Expect(cliArgs.SourceImage).To(Equal("ubuntu:24.04"))
	})

	It("resolves Mode=artifact by reading Artifact.ContainerImage", func() {
		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-2", Name: "ts", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{Mode: "artifact", SourceArtifactID: "a-1"},
		})
		Expect(err).ToNot(HaveOccurred())
		rec := awaitReady("e-2")
		Expect(rec.ContainerImage).To(Equal("quay.io/myorg/edge-os:v4.1.0"))
		Expect(dbCalls.Load()).To(Equal(int32(0)))
	})

	It("docker-builds Mode=dockerfile and uses the resulting tag", func() {
		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-3", Name: "ts", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{
				Mode: "dockerfile", Dockerfile: "FROM ubuntu:24.04\nRUN apt-get install -y curl",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		rec := awaitReady("e-3")
		Expect(dbCalls.Load()).To(Equal(int32(1)))
		argsMu.Lock()
		defer argsMu.Unlock()
		Expect(dbArgs.Tag).To(Equal("auroraboot-extbuild:e-3"))
		Expect(dbArgs.DockerfilePath).To(Equal(filepath.Join(baseDir, "e-3", "Dockerfile")))
		Expect(rec.ContainerImage).To(Equal("auroraboot-extbuild:e-3"))
	})

	It("docker-builds Mode=artifact with ExtraSteps and uses the resulting tag", func() {
		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-4", Name: "ts", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{
				Mode: "artifact", SourceArtifactID: "a-1",
				ExtraSteps: "RUN curl -fsSL https://tailscale.com/install.sh | sh",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		rec := awaitReady("e-4")
		Expect(dbCalls.Load()).To(Equal(int32(1)))

		argsMu.Lock()
		defer argsMu.Unlock()
		df, _ := os.ReadFile(dbArgs.DockerfilePath)
		Expect(string(df)).To(ContainSubstring("FROM quay.io/myorg/edge-os:v4.1.0"))
		Expect(string(df)).To(ContainSubstring("RUN curl -fsSL https://tailscale.com/install.sh"))
		Expect(rec.ContainerImage).To(Equal("auroraboot-extbuild:e-4"))
	})

	It("transitions to Error when source resolution fails", func() {
		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-5", Name: "ts", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{Mode: "artifact", SourceArtifactID: "does-not-exist"},
		})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() string {
			rec, _ := extStore.GetByID(context.Background(), "e-5")
			if rec == nil {
				return ""
			}
			return rec.Phase
		}, "2s", "20ms").Should(Equal(builder.BuildError))
	})
})

var _ = Describe("ExtensionBuilder.Build — auroraboot CLI invocation", func() {
	var (
		eb       *auroraboot.ExtensionBuilder
		extStore *fakeExtStore
		cliArgs  auroraboot.AurorabootCLIArgs
		argsMu   sync.Mutex
	)

	BeforeEach(func() {
		extStore = newFakeExtStore()
		eb = auroraboot.NewExtensionBuilder(GinkgoT().TempDir(), extStore).
			WithDockerBuildFunc(func(context.Context, auroraboot.DockerBuildArgs) error { return nil }).
			WithAurorabootCLIFunc(func(_ context.Context, a auroraboot.AurorabootCLIArgs) error {
				argsMu.Lock()
				cliArgs = a
				argsMu.Unlock()
				return os.WriteFile(filepath.Join(a.OutputDir, a.Name+"."+a.Type+".raw"), []byte("fake"), 0o644)
			})
	})

	awaitReady := func(id string) *store.ExtensionRecord {
		Eventually(func() string {
			rec, _ := extStore.GetByID(context.Background(), id)
			if rec == nil {
				return ""
			}
			return rec.Phase
		}, "2s", "20ms").Should(Equal(builder.BuildReady))
		rec, _ := extStore.GetByID(context.Background(), id)
		return rec
	}

	It("passes type/name/arch/output through to the CLI", func() {
		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-1", Name: "tailscale-agent", Type: "sysext", Arch: "amd64", Version: "v1.74.0",
			Source: builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
		})
		Expect(err).ToNot(HaveOccurred())
		rec := awaitReady("e-1")
		Expect(rec.RawFilename).To(Equal("tailscale-agent.sysext.raw"))
		argsMu.Lock()
		defer argsMu.Unlock()
		Expect(cliArgs.Type).To(Equal("sysext"))
		Expect(cliArgs.Name).To(Equal("tailscale-agent"))
		Expect(cliArgs.Arch).To(Equal("amd64"))
		Expect(cliArgs.SourceImage).To(Equal("ubuntu:24.04"))
	})

	It("passes hierarchies and ServiceReload for sysext", func() {
		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-2", Name: "ts", Type: "sysext", Arch: "amd64",
			Source:        builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
			Hierarchies:   []string{"/opt", "/srv"},
			ServiceReload: true,
		})
		Expect(err).ToNot(HaveOccurred())
		_ = awaitReady("e-2")
		argsMu.Lock()
		defer argsMu.Unlock()
		Expect(cliArgs.IncludePaths).To(Equal([]string{"/opt", "/srv"}))
		Expect(cliArgs.ServiceReload).To(BeTrue())
	})

	It("forwards signing files", func() {
		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-3", Name: "ts", Type: "sysext", Arch: "amd64",
			Source:  builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
			Signing: builder.ExtensionSigning{PrivateKey: "/tmp/db.key", Certificate: "/tmp/db.pem"},
		})
		Expect(err).ToNot(HaveOccurred())
		_ = awaitReady("e-3")
		argsMu.Lock()
		defer argsMu.Unlock()
		Expect(cliArgs.PrivateKey).To(Equal("/tmp/db.key"))
		Expect(cliArgs.Certificate).To(Equal("/tmp/db.pem"))
	})

	It("transitions to Error when the CLI fails", func() {
		eb = eb.WithAurorabootCLIFunc(func(context.Context, auroraboot.AurorabootCLIArgs) error {
			return fmt.Errorf("systemd-repart: device too small for verity")
		})
		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-4", Name: "ts", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
		})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() string {
			rec, _ := extStore.GetByID(context.Background(), "e-4")
			if rec == nil {
				return ""
			}
			return rec.Phase
		}, "2s", "20ms").Should(Equal(builder.BuildError))
	})
})

var _ = Describe("ExtensionBuilder — Status, List, Cancel", func() {
	var (
		eb       *auroraboot.ExtensionBuilder
		extStore *fakeExtStore
	)

	BeforeEach(func() {
		extStore = newFakeExtStore()
		eb = auroraboot.NewExtensionBuilder(GinkgoT().TempDir(), extStore).
			WithDockerBuildFunc(func(context.Context, auroraboot.DockerBuildArgs) error { return nil }).
			WithAurorabootCLIFunc(func(_ context.Context, a auroraboot.AurorabootCLIArgs) error {
				return os.WriteFile(filepath.Join(a.OutputDir, a.Name+"."+a.Type+".raw"), []byte("fake"), 0o644)
			})
	})

	It("Status returns the in-memory state for an existing build", func() {
		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-1", Name: "ts", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
		})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() string {
			st, _ := eb.Status(context.Background(), "e-1")
			if st == nil {
				return ""
			}
			return st.Phase
		}, "2s", "20ms").Should(Equal(builder.BuildReady))
	})

	It("List returns one status per known build", func() {
		for _, id := range []string{"e-a", "e-b", "e-c"} {
			_, _ = eb.Build(context.Background(), builder.ExtensionBuildOptions{
				ID: id, Name: id, Type: "sysext", Arch: "amd64",
				Source: builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
			})
		}
		Eventually(func() int {
			list, _ := eb.List(context.Background())
			return len(list)
		}, "2s", "20ms").Should(Equal(3))
	})

	It("Cancel transitions a running build to Error", func() {
		// Block the CLI seam so we have time to cancel.
		blocker := make(chan struct{})
		eb = eb.WithAurorabootCLIFunc(func(ctx context.Context, _ auroraboot.AurorabootCLIArgs) error {
			select {
			case <-blocker:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		_, _ = eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "e-c", Name: "ts", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
		})
		// Wait for Building phase to be reached.
		Eventually(func() string {
			st, _ := eb.Status(context.Background(), "e-c")
			if st == nil {
				return ""
			}
			return st.Phase
		}, "1s", "20ms").Should(Equal(builder.BuildBuilding))

		Expect(eb.Cancel(context.Background(), "e-c")).To(Succeed())
		close(blocker)
		Eventually(func() string {
			st, _ := eb.Status(context.Background(), "e-c")
			if st == nil {
				return ""
			}
			return st.Phase
		}, "2s", "20ms").Should(Equal(builder.BuildError))
	})

	It("Status returns 'not found' for an unknown ID", func() {
		_, err := eb.Status(context.Background(), "does-not-exist")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("ExtensionBuilder — log streaming", func() {
	It("streams CLI seam output into ExtensionStore.AppendLog", func() {
		extStore := newFakeExtStore()
		eb := auroraboot.NewExtensionBuilder(GinkgoT().TempDir(), extStore).
			WithDockerBuildFunc(func(context.Context, auroraboot.DockerBuildArgs) error { return nil }).
			WithAurorabootCLIFunc(func(_ context.Context, a auroraboot.AurorabootCLIArgs) error {
				if a.Logger != nil {
					_, _ = fmt.Fprintln(a.Logger, "building extension... ok")
				}
				return os.WriteFile(filepath.Join(a.OutputDir, a.Name+"."+a.Type+".raw"), []byte("fake"), 0o644)
			})

		_, err := eb.Build(context.Background(), builder.ExtensionBuildOptions{
			ID: "log-1", Name: "ts", Type: "sysext", Arch: "amd64",
			Source: builder.ExtensionSource{Mode: "image", BaseImage: "ubuntu:24.04"},
		})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() string {
			rec, _ := extStore.GetByID(context.Background(), "log-1")
			if rec == nil {
				return ""
			}
			return rec.Logs
		}, "2s", "20ms").Should(ContainSubstring("building extension... ok"))

		// Make sure we eventually wait until the build finishes so the goroutine
		// can flush before the test exits.
		Eventually(func() string {
			rec, _ := extStore.GetByID(context.Background(), "log-1")
			if rec == nil {
				return ""
			}
			return rec.Phase
		}, "2s", "20ms").Should(Equal(builder.BuildReady))
		_ = time.Now()
	})
})
