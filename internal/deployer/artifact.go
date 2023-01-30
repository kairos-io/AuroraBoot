package deployer

import (
	"fmt"
	"net/url"
	"path"
)

type ReleaseArtifact struct {
	ArtifactVersion string
	ReleaseVersion  string
	Flavor          string
	Repository      string
}

func urlBase(target string) (string, error) {
	u, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	return path.Base(u.Path), nil
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
