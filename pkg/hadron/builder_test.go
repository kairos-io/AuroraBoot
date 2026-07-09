package hadron

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordedInvocation captures a single ExecFunc call so tests can assert on
// argv shape, env visibility and log routing.
type recordedInvocation struct {
	name string
	args []string
	env  []string
}

// newRecorder returns an ExecFunc that stores every invocation and returns the
// provided per-call errors in order (nil = success). It writes a marker to
// stdout so the caller can prove log routing works.
func newRecorder(errs ...error) (*[]recordedInvocation, ExecFunc) {
	var got []recordedInvocation
	var i int
	fn := func(ctx context.Context, name string, args []string, env []string, stdout, stderr io.Writer) error {
		got = append(got, recordedInvocation{name: name, args: append([]string{}, args...), env: append([]string{}, env...)})
		fmt.Fprintf(stdout, "invocation %d: %s\n", i, strings.Join(args, " "))
		var err error
		if i < len(errs) {
			err = errs[i]
		}
		i++
		return err
	}
	return &got, fn
}

func TestBuild_PushOnly(t *testing.T) {
	tmp := t.TempDir()
	got, execFn := newRecorder(nil)
	var buf bytes.Buffer
	spec := Spec{
		BaseImage: "ghcr.io/kairos-io/hadron:main",
		Layers:    []string{"ghcr.io/kairos-io/git:latest"},
		Platforms: []string{"linux/amd64"},
		OutputRef: "example.com/team/os:v1",
		Push:      true,
	}
	res, err := Build(context.Background(), spec, tmp, nil, execFn, &buf)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if res.TarballPath != "" {
		t.Fatalf("no tarball expected, got %q", res.TarballPath)
	}
	if res.ImageRef != "example.com/team/os:v1" {
		t.Fatalf("unexpected image ref: %q", res.ImageRef)
	}
	if len(*got) != 1 {
		t.Fatalf("expected 1 buildx call, got %d", len(*got))
	}
	call := (*got)[0]
	if call.name != "docker" {
		t.Fatalf("expected docker, got %q", call.name)
	}
	joined := strings.Join(call.args, " ")
	for _, want := range []string{
		"buildx build",
		"--file " + filepath.Join(tmp, "Dockerfile.hadron"),
		"--platform linux/amd64",
		"--tag example.com/team/os:v1",
		"--push",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, "--output") {
		t.Fatalf("push-only build should not carry --output: %q", joined)
	}
	// Dockerfile written and matches renderer.
	body, err := os.ReadFile(res.DockerfilePath)
	if err != nil {
		t.Fatalf("read dockerfile: %v", err)
	}
	if want := RenderDockerfile(spec); string(body) != want {
		t.Fatalf("dockerfile mismatch: got %q want %q", body, want)
	}
	if !strings.Contains(buf.String(), "invocation 0") {
		t.Fatalf("expected log routing, got %q", buf.String())
	}
}

// buildxCall returns the args of the first invocation that starts with "buildx
// build" so tests don't have to skip past the qemu-binfmt install that Build
// runs before cross-arch invocations.
func buildxCall(t *testing.T, got []recordedInvocation) []string {
	t.Helper()
	for _, c := range got {
		if len(c.args) >= 2 && c.args[0] == "buildx" && c.args[1] == "build" {
			return c.args
		}
	}
	t.Fatalf("no buildx build call found; got %v", got)
	return nil
}

func TestBuild_TarballOnly_NoAuthNeeded(t *testing.T) {
	tmp := t.TempDir()
	// authProvider MUST NOT be invoked when push=false, so a panicking one
	// pins this contract: tarball-only builds work without credentials.
	authProvider := func(ctx context.Context) ([]RegistryCredential, error) {
		t.Fatalf("authProvider called for tarball-only build")
		return nil, nil
	}
	got, execFn := newRecorder(nil, nil)
	spec := Spec{
		BaseImage:      "ghcr.io/kairos-io/hadron:main",
		Platforms:      []string{"linux/amd64", "linux/arm64"},
		OutputRef:      "example.com/team/os:v1",
		ProduceTarball: true,
	}
	res, err := Build(context.Background(), spec, tmp, authProvider, execFn, io.Discard)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if res.TarballPath != filepath.Join(tmp, tarballFile) {
		t.Fatalf("unexpected tarball path: %q", res.TarballPath)
	}
	joined := strings.Join(buildxCall(t, *got), " ")
	for _, want := range []string{
		"--platform linux/amd64,linux/arm64",
		"type=oci",
		"dest=" + res.TarballPath,
		"name=example.com/team/os:v1",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, "--push") {
		t.Fatalf("tarball-only build should not carry --push: %q", joined)
	}
}

// TestBuild_CrossArch_InstallsBinfmt pins the qemu registration seam so a
// future refactor doesn't silently drop it and reintroduce "exec format
// error" on cross-arch builds.
func TestBuild_CrossArch_InstallsBinfmt(t *testing.T) {
	tmp := t.TempDir()
	got, execFn := newRecorder(nil, nil)
	spec := Spec{
		BaseImage:      "ghcr.io/kairos-io/hadron:main",
		Platforms:      []string{"linux/amd64", "linux/arm64"},
		OutputRef:      "example.com/team/os:v1",
		ProduceTarball: true,
	}
	if _, err := Build(context.Background(), spec, tmp, nil, execFn, io.Discard); err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if len(*got) < 2 {
		t.Fatalf("expected at least 2 exec calls (binfmt install + buildx), got %d", len(*got))
	}
	first := strings.Join((*got)[0].args, " ")
	if !strings.Contains(first, "tonistiigi/binfmt") || !strings.Contains(first, "--install") {
		t.Fatalf("expected first invocation to install binfmt, got %q", first)
	}
}

func TestBuild_PushAndTarball_PushRunsFirst(t *testing.T) {
	tmp := t.TempDir()
	// Push fails; tarball should not run.
	got, execFn := newRecorder(errors.New("boom"))
	spec := Spec{
		BaseImage:      "ghcr.io/kairos-io/hadron:main",
		Platforms:      []string{"linux/amd64"},
		OutputRef:      "example.com/team/os:v1",
		Push:           true,
		ProduceTarball: true,
	}
	_, err := Build(context.Background(), spec, tmp, nil, execFn, io.Discard)
	if err == nil {
		t.Fatalf("expected push failure to surface")
	}
	if len(*got) != 1 {
		t.Fatalf("expected only the push call, got %d", len(*got))
	}
	if !strings.Contains(strings.Join((*got)[0].args, " "), "--push") {
		t.Fatalf("first call should have been --push")
	}
}

func TestBuild_WritesDockerConfigForPush(t *testing.T) {
	tmp := t.TempDir()
	_, execFn := newRecorder(nil)
	authProvider := func(ctx context.Context) ([]RegistryCredential, error) {
		return []RegistryCredential{{
			Registry: "registry.example.com",
			Username: "alice",
			Password: "s3cret",
		}}, nil
	}
	spec := Spec{
		BaseImage: "ghcr.io/kairos-io/hadron:main",
		Platforms: []string{"linux/amd64"},
		OutputRef: "registry.example.com/team/os:v1",
		Push:      true,
	}
	if _, err := Build(context.Background(), spec, tmp, authProvider, execFn, io.Discard); err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	cfgPath := filepath.Join(tmp, "docker", "config.json")
	body, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("expected docker config at %s: %v", cfgPath, err)
	}
	var parsed struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse docker config: %v", err)
	}
	entry, ok := parsed.Auths["registry.example.com"]
	if !ok {
		t.Fatalf("expected auth entry for registry.example.com, got %v", parsed.Auths)
	}
	want := base64.StdEncoding.EncodeToString([]byte("alice:s3cret"))
	if entry.Auth != want {
		t.Fatalf("auth mismatch: got %q want %q", entry.Auth, want)
	}
}

// TestBuild_NoCacheFlag pins the --no-cache plumbing: the flag reaches buildx
// exactly when the spec asks for it, and never sneaks in on default builds
// (which would wreck cache hits for the common re-run case).
func TestBuild_NoCacheFlag(t *testing.T) {
	cases := []struct {
		name    string
		noCache bool
		want    bool
	}{
		{name: "NoCache=true adds --no-cache", noCache: true, want: true},
		{name: "NoCache=false omits --no-cache", noCache: false, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			got, execFn := newRecorder(nil, nil)
			spec := Spec{
				BaseImage:      "ghcr.io/kairos-io/hadron:main",
				Platforms:      []string{"linux/amd64"},
				OutputRef:      "example.com/team/os:v1",
				Push:           true,
				ProduceTarball: true,
				NoCache:        tc.noCache,
			}
			if _, err := Build(context.Background(), spec, tmp, nil, execFn, io.Discard); err != nil {
				t.Fatalf("Build failed: %v", err)
			}
			if len(*got) != 2 {
				t.Fatalf("expected push+tarball calls, got %d", len(*got))
			}
			for i, call := range *got {
				has := false
				for _, a := range call.args {
					if a == "--no-cache" {
						has = true
						break
					}
				}
				if has != tc.want {
					t.Fatalf("call %d: --no-cache present=%v want=%v (args: %s)", i, has, tc.want, strings.Join(call.args, " "))
				}
			}
		})
	}
}

func TestBuild_InvalidSpec(t *testing.T) {
	_, err := Build(context.Background(), Spec{}, t.TempDir(), nil, nil, io.Discard)
	if err == nil || !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("expected ErrInvalidSpec, got %v", err)
	}
}
