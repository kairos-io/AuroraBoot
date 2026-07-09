package hadron

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// RegistryCredential is a single (registry, username, password) triplet used to
// authenticate `docker buildx build --push` against a private registry. The
// server persists these encrypted in the settings store and hands the decrypted
// list to Build via a RegistryAuthProvider callback.
type RegistryCredential struct {
	Registry string
	Username string
	Password string
}

// RegistryAuthProvider returns the credentials to make available to a build.
// Returning an empty slice (or a nil error with no creds) is fine — buildx just
// won't have any auth material and public pulls/pushes still work.
type RegistryAuthProvider func(ctx context.Context) ([]RegistryCredential, error)

// ExecFunc is the seam Build uses to spawn buildx. Tests inject a recorder;
// production hands over runExec, which is a thin wrapper around os/exec.
type ExecFunc func(ctx context.Context, name string, args []string, env []string, stdout, stderr io.Writer) error

// Result carries the outputs of a successful Build.
type Result struct {
	// ImageRef is the tag applied to the built image. Matches Spec.OutputRef.
	ImageRef string
	// TarballPath is set when Spec.ProduceTarball was true. Absolute path
	// on disk pointing at the OCI archive written into workDir.
	TarballPath string
	// DockerfilePath is the on-disk path of the generated Dockerfile, kept
	// around so the UI can render it in the artifact detail view.
	DockerfilePath string
}

// tarballFile is the fixed name used inside workDir for the OCI export. Kept
// stable so the artifacts download endpoint can serve it by name.
const tarballFile = "hadron.oci.tar"

// dockerfileFile is the fixed name of the generated Dockerfile inside workDir.
const dockerfileFile = "Dockerfile.hadron"

// Build renders the Dockerfile, prepares an isolated docker config (from
// authProvider, when push=true) and invokes buildx once per requested output
// mode. Output streams to logWriter as buildx runs, so callers can plumb it
// into a live log broadcaster the same way the existing Kairos build does.
//
// Multi-platform builds cannot be `--load`ed into a local daemon; the two
// legal modes are push OR OCI-tarball export. Both may be requested together —
// buildx is invoked twice, sharing the same layer cache.
//
// The returned *Result is non-nil whenever the Dockerfile was rendered and
// written to disk, even on subsequent error. Callers can rely on
// Result.DockerfilePath to expose the generated Dockerfile so operators can
// reproduce a failed build outside AuroraBoot.
func Build(ctx context.Context, spec Spec, workDir string, authProvider RegistryAuthProvider, execFn ExecFunc, logWriter io.Writer) (*Result, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if execFn == nil {
		execFn = runExec
	}

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating hadron work dir: %w", err)
	}

	dockerfilePath := filepath.Join(workDir, dockerfileFile)
	if err := os.WriteFile(dockerfilePath, []byte(RenderDockerfile(spec)), 0o644); err != nil {
		return nil, fmt.Errorf("writing hadron Dockerfile: %w", err)
	}
	// From this point onward the Dockerfile is on disk. Every error path
	// returns a partial Result so the caller can surface the file to the UI.
	partial := &Result{
		ImageRef:       spec.OutputRef,
		DockerfilePath: dockerfilePath,
	}

	// Empty build context — the Dockerfile only references remote images and
	// never `COPY`s from local paths, so a stub dir is enough for buildx.
	ctxDir := filepath.Join(workDir, "context")
	if err := os.MkdirAll(ctxDir, 0o755); err != nil {
		return partial, fmt.Errorf("creating hadron build context: %w", err)
	}

	// Auth is only needed for push. Skipping it entirely for tarball-only
	// builds means operators without a registry can still produce images.
	extraEnv := os.Environ()
	if spec.Push && authProvider != nil {
		dockerCfgDir, err := writeDockerConfig(workDir, authProvider, ctx)
		if err != nil {
			return partial, fmt.Errorf("preparing docker auth: %w", err)
		}
		if dockerCfgDir != "" {
			extraEnv = append(extraEnv, "DOCKER_CONFIG="+dockerCfgDir)
		}
	}

	// Cross-arch RUN steps inside buildx execute under qemu-user emulation, which
	// requires the host to have binfmt_misc entries for the target architecture.
	// A missing entry surfaces as the notoriously-opaque "exec format error"
	// mid-build. tonistiigi/binfmt is the community-standard installer; running
	// it before buildx is a no-op when the registration already exists, so we do
	// it unconditionally whenever the requested platform set touches a non-host
	// architecture. Failure is downgraded to a warning — some hosts (rootless,
	// unprivileged, or already-configured) won't accept the install and we let
	// the actual build proceed and report whatever error buildx sees.
	platforms := spec.PlatformsOrDefault()
	if needsEmulation(platforms) {
		installArgs := []string{"run", "--privileged", "--rm", "tonistiigi/binfmt", "--install", crossArches(platforms)}
		if err := runBuildx(ctx, execFn, installArgs, extraEnv, logWriter); err != nil && logWriter != nil {
			fmt.Fprintf(logWriter, "note: qemu binfmt install failed (%v). Cross-arch RUN steps may still fail with 'exec format error'; register binfmt on the host manually if so.\n", err)
		}
	}

	baseArgs := []string{
		"buildx", "build",
		"--file", dockerfilePath,
		"--platform", strings.Join(platforms, ","),
		"--tag", spec.OutputRef,
	}

	// Push first when both modes are requested: it's the interactive-facing
	// side effect, and any auth failure is surfaced before we spend cycles
	// producing a tarball nobody can consume.
	if spec.Push {
		args := append([]string{}, baseArgs...)
		args = append(args, "--push", ctxDir)
		if err := runBuildx(ctx, execFn, args, extraEnv, logWriter); err != nil {
			return partial, fmt.Errorf("buildx push: %w", err)
		}
	}

	if spec.ProduceTarball {
		tarballPath := filepath.Join(workDir, tarballFile)
		args := append([]string{}, baseArgs...)
		args = append(args,
			"--output", fmt.Sprintf("type=oci,dest=%s,name=%s", tarballPath, spec.OutputRef),
			ctxDir,
		)
		if err := runBuildx(ctx, execFn, args, extraEnv, logWriter); err != nil {
			return partial, fmt.Errorf("buildx tarball export: %w", err)
		}
		partial.TarballPath = tarballPath
	}

	return partial, nil
}

// runBuildx wraps ExecFunc with a small log preamble so build transcripts show
// exactly what buildx was invoked with. Credentials never appear in this
// preamble — they live in DOCKER_CONFIG, not on the command line.
func runBuildx(ctx context.Context, execFn ExecFunc, args, env []string, logWriter io.Writer) error {
	if logWriter != nil {
		fmt.Fprintf(logWriter, "$ docker %s\n", strings.Join(args, " "))
	}
	return execFn(ctx, "docker", args, env, logWriter, logWriter)
}

// writeDockerConfig materializes a docker CLI config directory populated with
// auth entries from the provider. When the provider returns no credentials we
// return an empty string, letting the caller skip DOCKER_CONFIG entirely.
//
// The config lives inside workDir/docker so it's cleaned up alongside the rest
// of the build's artifacts, and so buildx cannot accidentally reach into the
// operator's real ~/.docker on a shared build host.
func writeDockerConfig(workDir string, authProvider RegistryAuthProvider, ctx context.Context) (string, error) {
	creds, err := authProvider(ctx)
	if err != nil {
		return "", err
	}
	if len(creds) == 0 {
		return "", nil
	}

	dir := filepath.Join(workDir, "docker")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	type authEntry struct {
		Auth string `json:"auth"`
	}
	auths := map[string]authEntry{}
	for _, c := range creds {
		if c.Registry == "" || c.Username == "" {
			continue
		}
		token := base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password))
		auths[c.Registry] = authEntry{Auth: token}
	}
	body, err := json.Marshal(map[string]any{"auths": auths})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), body, 0o600); err != nil {
		return "", err
	}
	return dir, nil
}

// runExec is the production ExecFunc. Kept small so tests can substitute a
// recorder without dragging os/exec into their expectations.
func runExec(ctx context.Context, name string, args []string, env []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// needsEmulation returns true if any requested platform names an architecture
// different from the host — those platforms will run RUN steps under qemu-user
// and therefore require a binfmt_misc registration on the host.
func needsEmulation(platforms []string) bool {
	host := runtime.GOARCH
	for _, p := range platforms {
		if _, arch, ok := splitPlatform(p); ok && arch != host {
			return true
		}
	}
	return false
}

// crossArches returns the comma-joined list of non-host architectures for
// tonistiigi/binfmt --install. Passing the exact list keeps the installer
// scoped so hosts that already have unrelated arches configured aren't
// disturbed.
func crossArches(platforms []string) string {
	host := runtime.GOARCH
	seen := map[string]bool{}
	var out []string
	for _, p := range platforms {
		if _, arch, ok := splitPlatform(p); ok && arch != host && !seen[arch] {
			seen[arch] = true
			out = append(out, arch)
		}
	}
	if len(out) == 0 {
		return "all"
	}
	return strings.Join(out, ",")
}

// splitPlatform parses a "linux/arm64"-style buildx platform string. Returns
// os, arch and whether the input was well-formed.
func splitPlatform(p string) (string, string, bool) {
	parts := strings.SplitN(p, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
