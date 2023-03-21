package auroraboot_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kairos-io/kairos/pkg/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var auroraBootImage = "auroraboot"

func TestSuite(t *testing.T) {
	envImage := os.Getenv("AURORABOOT_IMAGE")
	if envImage != "" {
		auroraBootImage = envImage
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "Auroraboot Test Suite")
}

func RunAurora(cmd, dir string) (string, error) {
	runCmd := fmt.Sprintf(`cd %s && docker run --privileged -v "$PWD"/config.yaml:/config.yaml -v "$PWD"/build:/tmp/auroraboot -v /var/run/docker.sock:/var/run/docker.sock --rm %s --debug %s`, dir, auroraBootImage, cmd)
	return utils.SH(runCmd)
}

func PullImage(image string) (string, error) {
	runCmd := fmt.Sprintf(`docker pull %s`, image)
	return utils.SH(runCmd)
}

func WriteConfig(config, dir string) error {
	os.RemoveAll(filepath.Join(dir, "config.yaml"))
	f, err := os.Create(filepath.Join(dir, "config.yaml"))
	if err != nil {
		return err
	}

	_, err = f.WriteString(config)
	return err
}
