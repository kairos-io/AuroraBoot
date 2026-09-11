package auroraboot

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// buildxPlatforms is the buildx probe, replaced in tests.
var buildxPlatforms = dockerBuildxPlatforms

// dockerBuildxPlatforms asks the active buildx builder which platforms it can
// build for. buildx lists emulated platforms next to native ones, so a missing
// entry means the daemon host has no binfmt_misc handler registered for it.
//
// Reading our own /proc/sys/fs/binfmt_misc would answer the wrong question:
// AuroraBoot usually runs as a container with /var/run/docker.sock bind-mounted,
// so the builds happen on a host we are not looking at.
func dockerBuildxPlatforms(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "docker", "buildx", "inspect").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker buildx inspect: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return parseBuildxPlatforms(string(out)), nil
}

// parseBuildxPlatforms reads the platform list out of "docker buildx inspect".
// The field repeats once per node, and buildx marks the platforms it prefers
// with a trailing asterisk:
//
//	Platforms:  linux/arm64*, linux/amd64, linux/arm/v7
func parseBuildxPlatforms(out string) []string {
	var platforms []string
	for _, line := range strings.Split(out, "\n") {
		_, list, found := strings.Cut(line, "Platforms:")
		if !found {
			continue
		}
		for _, platform := range strings.Split(list, ",") {
			platform = strings.TrimSuffix(strings.TrimSpace(platform), "*")
			if platform != "" {
				platforms = append(platforms, platform)
			}
		}
	}
	return platforms
}

// checkBuildPlatform refuses a cross-architecture build up front when the
// docker builder cannot execute the requested architecture.
//
// Passing --platform is enough to resolve the base image manifest, but the
// kairosify step then runs the target image's own binaries (/kairos-init), and
// a custom Dockerfile's RUN steps do the same. That needs binfmt_misc emulation
// on the daemon host. Without this check the build gets past the manifest and
// dies much later with an exec format error that names neither the
// architecture nor the missing emulator. See kairos-io/kairos#4088.
func checkBuildPlatform(ctx context.Context, arch string) error {
	if arch == "" {
		return nil
	}
	platforms, err := buildxPlatforms(ctx)
	// The probe is advisory. Only refuse on positive evidence that the
	// platform is missing: a docker without buildx, or a daemon that does not
	// answer, must not block a build that would otherwise have worked.
	if err != nil || len(platforms) == 0 {
		return nil
	}

	want := "linux/" + arch
	for _, platform := range platforms {
		// A variant counts as support: linux/arm/v7 can build linux/arm.
		if platform == want || strings.HasPrefix(platform, want+"/") {
			return nil
		}
	}

	return fmt.Errorf("the docker builder cannot build for %s, it offers %s. "+
		"A cross-architecture build runs the target image's own binaries, so the host running "+
		"the docker daemon needs binfmt_misc emulation registered for %s: "+
		"docker run --privileged --rm tonistiigi/binfmt --install %s",
		want, strings.Join(platforms, ", "), arch, arch)
}
