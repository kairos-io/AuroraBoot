package deployer

import (
	"fmt"
)

type ReleaseArtifact struct {
	ArtifactVersion string `yaml:"artifact_version"`
	ReleaseVersion  string `yaml:"release_version"`
	Flavor          string `yaml:"flavor"`
	Repository      string `yaml:"repository"`

	ContainerImage string `yaml:"container_image"`
}

func urlGen(repository, releaseVersion, flavor, artifactVersion, artifactType string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repository, releaseVersion, fileName(flavor, artifactVersion, artifactType))
}

func fileName(flavor, artifactVersion, artifactType string) string {
	return fmt.Sprintf("kairos-%s-%s%s", flavor, artifactVersion, artifactType)
}

func (a ReleaseArtifact) NetbootArtifacts() []string {

	return []string{a.InitrdURL()}
}
func (a ReleaseArtifact) ISOUrl() string {
	return urlGen(a.Repository, a.ReleaseVersion, a.Flavor, a.ArtifactVersion, ".iso")
}

func (a ReleaseArtifact) InitrdURL() string {
	return urlGen(a.Repository, a.ReleaseVersion, a.Flavor, a.ArtifactVersion, "-initrd")
}

func (a ReleaseArtifact) KernelURL() string {
	return urlGen(a.Repository, a.ReleaseVersion, a.Flavor, a.ArtifactVersion, "-kernel")
}

func (a ReleaseArtifact) SquashFSURL() string {
	return urlGen(a.Repository, a.ReleaseVersion, a.Flavor, a.ArtifactVersion, ".squashfs")
}
