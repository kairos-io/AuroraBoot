// Package hadron builds Hadron OS images by composing a base Hadron image with
// zero or more firmware layers (from ghcr.io/kairos-io/hadron-firmware) and
// software layers (from ghcr.io/kairos-io/hadron-layers) via a generated
// Dockerfile driven by `docker buildx build`.
//
// The output is a plain OCI image: no Kairos-specific wrapping, no signing.
// Callers that want a bootable Kairos image chain the result through the
// existing kairosify path (kairos-init as a build stage).
package hadron

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Spec is a self-contained description of a Hadron image build.
//
// JSON tags are lowerCamelCase so the persisted HadronSpecJSON blob matches
// the wire format used everywhere else in the API — the UI parses these
// fields directly out of the artifact record.
type Spec struct {
	// BaseImage is the Hadron base ref, e.g. "ghcr.io/kairos-io/hadron:main".
	BaseImage string `json:"baseImage"`

	// Firmware is the list of firmware image refs to merge into the base.
	// Each ref is copied whole into the image root via `COPY --from=<ref> / /`.
	// Order matters only when two firmware images ship the same file: the
	// later ref wins. Values are used verbatim, so they must include the tag.
	Firmware []string `json:"firmware,omitempty"`

	// Layers is the list of software layer image refs to merge into the base.
	// Same COPY semantics as Firmware. Later entries can overwrite earlier
	// entries, so the caller controls precedence.
	Layers []string `json:"layers,omitempty"`

	// ExtraDockerfile is a free-form Dockerfile fragment appended verbatim
	// after the generated FROM/COPY lines. Runs as part of the same buildx
	// invocation. Kept optional to support one-off RUN/COPY tweaks without
	// forcing users out to a wrapper repo.
	ExtraDockerfile string `json:"extraDockerfile,omitempty"`

	// Platforms is the list of buildx --platform values (e.g. "linux/amd64",
	// "linux/arm64"). Empty defaults to a single amd64 build.
	Platforms []string `json:"platforms,omitempty"`

	// OutputRef is the image tag to apply to the built image, e.g.
	// "registry.example.com/team/hadron:v1". Required whenever Push or
	// ProduceTarball is set — buildx needs an image name for both destinations.
	OutputRef string `json:"outputRef"`

	// Push, when true, invokes buildx with `--push` so the image is uploaded
	// to the registry implied by OutputRef.
	Push bool `json:"push,omitempty"`

	// ProduceTarball, when true, exports the built image to an OCI tarball on
	// disk (written into the build's working directory as `hadron.oci.tar`).
	ProduceTarball bool `json:"produceTarball,omitempty"`
}

// imageRefRe matches an image reference we're willing to embed in a Dockerfile.
// It deliberately rejects shell metacharacters — anything that could break out
// of the `COPY --from=<ref>` context — while accepting every plausible ghcr /
// quay / docker.io reference. Digests are supported via the `@sha256:...` form.
var imageRefRe = regexp.MustCompile(`^[A-Za-z0-9._:/@+-]+$`)

// ErrInvalidSpec marks any validation failure on a build spec. Handlers can
// errors.Is on it to map to a 400 Bad Request.
var ErrInvalidSpec = errors.New("invalid hadron spec")

// validPlatforms is the set of buildx --platform values we accept for now.
// Extending this list is a code change on purpose — we don't want operators
// smuggling arbitrary strings into buildx.
var validPlatforms = map[string]struct{}{
	"linux/amd64": {},
	"linux/arm64": {},
}

// Validate rejects specs that cannot be built. Callers should call this before
// persisting the record or invoking Build.
func (s *Spec) Validate() error {
	if strings.TrimSpace(s.BaseImage) == "" {
		return fmt.Errorf("%w: baseImage is required", ErrInvalidSpec)
	}
	if !imageRefRe.MatchString(s.BaseImage) {
		return fmt.Errorf("%w: baseImage %q has invalid characters", ErrInvalidSpec, s.BaseImage)
	}
	for i, ref := range s.Firmware {
		if !imageRefRe.MatchString(ref) {
			return fmt.Errorf("%w: firmware[%d] %q has invalid characters", ErrInvalidSpec, i, ref)
		}
	}
	for i, ref := range s.Layers {
		if !imageRefRe.MatchString(ref) {
			return fmt.Errorf("%w: layers[%d] %q has invalid characters", ErrInvalidSpec, i, ref)
		}
	}
	for _, p := range s.Platforms {
		if _, ok := validPlatforms[p]; !ok {
			return fmt.Errorf("%w: platform %q not supported (allowed: linux/amd64, linux/arm64)", ErrInvalidSpec, p)
		}
	}
	if !s.Push && !s.ProduceTarball {
		return fmt.Errorf("%w: at least one of push or produceTarball must be set", ErrInvalidSpec)
	}
	if s.OutputRef == "" {
		return fmt.Errorf("%w: outputRef is required when push or produceTarball is set", ErrInvalidSpec)
	}
	if !imageRefRe.MatchString(s.OutputRef) {
		return fmt.Errorf("%w: outputRef %q has invalid characters", ErrInvalidSpec, s.OutputRef)
	}
	return nil
}

// PlatformsOrDefault returns the effective platform list — the spec's own list
// when set, or a single "linux/amd64" entry as fallback.
func (s *Spec) PlatformsOrDefault() []string {
	if len(s.Platforms) == 0 {
		return []string{"linux/amd64"}
	}
	return s.Platforms
}
