package schema

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type ReleaseArtifact struct {
	ArtifactVersion string `yaml:"artifact_version"`
	Model           string `yaml:"model"`
	Flavor          string `yaml:"flavor"`
	Platform        string `yaml:"platform"`
	ReleaseVersion  string `yaml:"release_version"`
	Repository      string `yaml:"repository"`
	Variant         string `yaml:"variant"`

	ContainerImage string `yaml:"container_image"`
}

func (a ReleaseArtifact) FileName() string {
	if a.ContainerImage != "" {
		return ""
	}

	if a.Model == "" {
		a.Model = "generic"
	}
	if a.Platform == "" {
		a.Platform = "amd64"
	}
	if a.Variant == "" {
		if strings.Contains(a.ArtifactVersion, "k3s") {
			a.Variant = "standard"
		} else {
			a.Variant = "core"
		}

	}

	var re = regexp.MustCompile(`v(?P<major>\d+)\.(?P<minor>\d+).+`)
	match := re.FindStringSubmatch(a.ReleaseVersion)
	result := make(map[string]string)
	for i, name := range re.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}

	major, _ := strconv.Atoi(result["major"])
	minor, _ := strconv.Atoi(result["minor"])

	if major < 2 || (major == 2 && minor < 4) {
		return fmt.Sprintf("kairos-%s-%s", a.Flavor, a.ArtifactVersion)
	}

	return fmt.Sprintf("kairos-%s-%s-%s-generic-%s", a.Variant, a.Flavor, a.Platform, a.ArtifactVersion)
}

func (a ReleaseArtifact) urlGen(ext string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s%s", a.Repository, a.ReleaseVersion, a.FileName(), ext)
}

func (a ReleaseArtifact) NetbootArtifacts() []string {
	return []string{a.InitrdURL()}
}

func (a ReleaseArtifact) ISOUrl() string {
	return a.urlGen(".iso")
}

func (a ReleaseArtifact) InitrdURL() string {
	return a.urlGen("-initrd")
}

func (a ReleaseArtifact) KernelURL() string {
	return a.urlGen("-kernel")
}

func (a ReleaseArtifact) SquashFSURL() string {
	return a.urlGen(".squashfs")
}
