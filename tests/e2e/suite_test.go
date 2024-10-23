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
	cmdOut, cmdErr := utils.SH(runCmd)
	if cmdErr != nil {
		return cmdOut, cmdErr
	}

	// Fix permissions of build directory to allow running tests locally rootless
	permCmd := fmt.Sprintf(`cd %s && docker run --privileged -e USERID=$(id -u) -e GROUPID=$(id -g) --entrypoint /usr/bin/sh -v "$PWD"/build:/tmp/auroraboot --rm %s -c 'chown -R $USERID:$GROUPID /tmp/auroraboot'`, dir, auroraBootImage)
	permOut, permErr := utils.SH(permCmd)
	Expect(permErr).ToNot(HaveOccurred(), permOut)

	return cmdOut, cmdErr
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
