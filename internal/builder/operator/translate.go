package operator

import (
	"fmt"

	buildv1alpha2 "github.com/kairos-io/kairos-operator/api/v1alpha2"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
)

// translateBuildOptions maps AuroraBoot BuildOptions to an OSArtifactSpec.
//
// Notes on quirks:
//   - Outputs.CloudImage and Outputs.RawDisk both map to Artifacts.CloudImage
//     because the operator has a single "cloud image" output; the local builder
//     historically distinguished the two.
//   - Outputs.Tar has no explicit toggle on the CRD; the operator always
//     writes /artifacts/<name>.tar as a byproduct of the buildah stage. Setting
//     Tar alone is therefore not an error even though no field carries it.
func translateBuildOptions(id string, opts builder.BuildOptions) (buildv1alpha2.OSArtifactSpec, error) {
	arch := opts.Source.Arch
	switch arch {
	case "", "amd64", "arm64":
	default:
		return buildv1alpha2.OSArtifactSpec{}, fmt.Errorf("%w: source.arch must be 'amd64' or 'arm64', got %q", builder.ErrInvalidBuildOptions, arch)
	}

	// FIPS and TrustedBoot are build-time flags passed to kairos-init; they
	// only make sense when we actually build the Stage-1 image. Rejecting them
	// on a pre-built ref makes the misconfiguration loud instead of silent.
	preBuilt := opts.BaseImage != "" && opts.Dockerfile == ""
	if preBuilt && opts.Outputs.FIPS {
		return buildv1alpha2.OSArtifactSpec{}, fmt.Errorf("%w: outputs.fips requires a from-scratch build, not a pre-built base image", builder.ErrInvalidBuildOptions)
	}
	if preBuilt && opts.Outputs.TrustedBoot {
		return buildv1alpha2.OSArtifactSpec{}, fmt.Errorf("%w: outputs.trustedBoot requires a from-scratch build, not a pre-built base image", builder.ErrInvalidBuildOptions)
	}

	spec := buildv1alpha2.OSArtifactSpec{}

	switch {
	case preBuilt:
		spec.Image = buildv1alpha2.ImageSpec{Ref: opts.BaseImage}
	case opts.Dockerfile != "":
		spec.Image = buildv1alpha2.ImageSpec{
			OCISpec: &buildv1alpha2.OCISpec{
				Ref: &buildv1alpha2.SecretKeySelector{
					Name: dockerfileSecretName(id),
					Key:  dockerfileSecretKey,
				},
			},
		}
	default:
		spec.Image = buildv1alpha2.ImageSpec{
			BuildOptions: &buildv1alpha2.BuildOptions{
				Version:           firstNonEmpty(opts.Source.KairosVersion, opts.KairosVersion),
				BaseImage:         opts.Source.BaseImage,
				Model:             firstNonEmpty(opts.Source.Model, opts.Model),
				TrustedBoot:       opts.Outputs.TrustedBoot || opts.TrustedBoot,
				KubernetesDistro:  firstNonEmpty(opts.Source.KubernetesDistro, opts.KubernetesDistro),
				KubernetesVersion: firstNonEmpty(opts.Source.KubernetesVersion, opts.KubernetesVersion),
				FIPS:              opts.Outputs.FIPS || opts.FIPS,
			},
		}
	}

	artifacts := &buildv1alpha2.ArtifactSpec{Arch: arch}
	if opts.Outputs.ISO || opts.ISO {
		artifacts.ISO = true
	}
	if opts.Outputs.CloudImage || opts.Outputs.RawDisk || opts.CloudImage {
		artifacts.CloudImage = true
	}
	if opts.Outputs.Netboot || opts.Netboot {
		artifacts.Netboot = true
	}
	if opts.Outputs.GCE {
		artifacts.GCEImage = true
	}
	if opts.Outputs.VHD {
		artifacts.AzureImage = true
	}
	if opts.Outputs.UKI {
		artifacts.UKI = &buildv1alpha2.UKISpec{ISO: true}
	}
	if opts.CloudConfig != "" {
		artifacts.CloudConfigRef = &buildv1alpha2.SecretKeySelector{
			Name: cloudConfigSecretName(id),
			Key:  cloudConfigSecretKey,
		}
	}
	spec.Artifacts = artifacts

	return spec, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
