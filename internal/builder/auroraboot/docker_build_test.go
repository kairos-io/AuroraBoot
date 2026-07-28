package auroraboot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

func TestDockerBuildPlatformArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arch string
		want []string
	}{
		{
			name: "target architecture",
			arch: "amd64",
			want: []string{"--platform", "linux/amd64"},
		},
		{
			name: "unspecified architecture",
			want: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := builder.BuildOptions{}
			opts.Source.Arch = tt.arch
			got := dockerBuildPlatformArgs(opts)

			if len(got) != len(tt.want) {
				t.Fatalf("dockerBuildPlatformArgs() = %q, want %q", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("dockerBuildPlatformArgs()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDockerBuildsUseTargetPlatform(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Builder, builder.BuildOptions, string) error
	}{
		{
			name: "custom Dockerfile",
			run: func(b *Builder, opts builder.BuildOptions, outputDir string) error {
				opts.Dockerfile = "FROM scratch\n"
				_, err := b.dockerBuild(context.Background(), opts, outputDir, nil)
				return err
			},
		},
		{
			name: "kairosification",
			run: func(b *Builder, opts builder.BuildOptions, outputDir string) error {
				_, err := b.ensureKairosified(context.Background(), "example.invalid/base:latest", opts, outputDir, nil)
				return err
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()
			callsPath := filepath.Join(baseDir, "docker.calls")
			dockerPath := filepath.Join(baseDir, "docker")
			script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + callsPath + "\"\n" +
				"if [ \"$1\" = inspect ] || [ \"$1\" = run ]; then exit 1; fi\n"
			if err := os.WriteFile(dockerPath, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", baseDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			b := New(baseDir, nil, noopArtifactStore{})
			opts := builder.BuildOptions{ID: "cross-arch"}
			opts.Source.Arch = "amd64"
			if err := tt.run(b, opts, baseDir); err != nil {
				t.Fatal(err)
			}

			calls, err := os.ReadFile(callsPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(calls), "build") ||
				!strings.Contains(string(calls), "--platform linux/amd64") {
				t.Fatalf("docker calls did not target linux/amd64:\n%s", calls)
			}
		})
	}
}

type noopArtifactStore struct{}

func (noopArtifactStore) Create(context.Context, *store.ArtifactRecord) error { return nil }
func (noopArtifactStore) GetByID(context.Context, string) (*store.ArtifactRecord, error) {
	return nil, nil
}
func (noopArtifactStore) List(context.Context) ([]*store.ArtifactRecord, error) { return nil, nil }
func (noopArtifactStore) Update(context.Context, *store.ArtifactRecord) error   { return nil }
func (noopArtifactStore) UpdatePhaseMessage(context.Context, string, string, string) error {
	return nil
}
func (noopArtifactStore) UpdateFiles(context.Context, string, []string) error { return nil }
func (noopArtifactStore) ClearUploadToken(context.Context, string) error      { return nil }
func (noopArtifactStore) Delete(context.Context, string) error                { return nil }
func (noopArtifactStore) DeleteByPhase(context.Context, string) error         { return nil }
func (noopArtifactStore) GetLogs(context.Context, string) (string, error)     { return "", nil }
func (noopArtifactStore) AppendLog(context.Context, string, string) error     { return nil }
