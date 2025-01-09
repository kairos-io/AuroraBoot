package e2e_test

import (
	"fmt"
	"github.com/kairos-io/kairos-sdk/utils"
	"github.com/onsi/gomega/types"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/spectrocloud/peg/matcher"
)

var getVersionCmd = ". /etc/kairos-release; [ ! -z \"$KAIROS_VERSION\" ] && echo $KAIROS_VERSION"

var stateAssertVM = func(vm VM, query, expected string) {
	By(fmt.Sprintf("Expecting state %s to be %s", query, expected))
	out, err := vm.Sudo(fmt.Sprintf("kairos-agent state get %s", query))
	ExpectWithOffset(1, err).ToNot(HaveOccurred(), out)
	ExpectWithOffset(1, out).To(ContainSubstring(expected))
}

var stateContains = func(vm VM, query string, expected ...string) {
	var or []types.GomegaMatcher
	for _, e := range expected {
		or = append(or, ContainSubstring(e))
	}
	out, err := vm.Sudo(fmt.Sprintf("kairos-agent state get %s", query))
	ExpectWithOffset(1, err).ToNot(HaveOccurred(), out)
	ExpectWithOffset(1, strings.ToLower(out)).To(Or(or...))
}

type Auroraboot struct {
	Path           string
	ContainerImage string
	Dirs           []string          // directories to mount from host
	ManualDirs     map[string]string // directories to mount from host to an specific path in the container
}

func TestAurorabootE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Auroraboot end to end test suite")
}

func NewAuroraboot(image string, dirs ...string) *Auroraboot {
	tmpDir, err := os.MkdirTemp("", "auroraboot-e2e-tmp")
	Expect(err).ToNot(HaveOccurred())
	aurorabootBinary := path.Join(tmpDir, "auroraboot")
	compileAuroraboot(aurorabootBinary)
	return &Auroraboot{ContainerImage: image, Path: aurorabootBinary, Dirs: dirs}
}

// auroraboot relies on various external binaries. To make sure those dependencies
// are in place (or to test the behavior of auroraboot when they are not), we run auroraboot
// in a container using this function.
func (e *Auroraboot) Run(aurorabootArgs ...string) (string, error) {
	return e.ContainerRun("/bin/auroraboot", aurorabootArgs...)
}

// We need --privileged for `mount` to work in the container (used in the build_uki_test.go).
func (e *Auroraboot) ContainerRun(entrypoint string, args ...string) (string, error) {
	dockerArgs := []string{
		"run", "--rm", "--privileged",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"--entrypoint", entrypoint,
		"-v", fmt.Sprintf("%s:/bin/auroraboot", e.Path),
	}

	for _, d := range e.Dirs {
		dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%[1]s:%[1]s", d))
	}

	for k, v := range e.ManualDirs {
		dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:%s", k, v))
	}

	dockerArgs = append(dockerArgs, e.ContainerImage)
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.Command("docker", dockerArgs...)
	out, err := cmd.CombinedOutput()

	return string(out), err
}

func (e *Auroraboot) Cleanup() {
	dir := filepath.Dir(e.Path)
	Expect(os.RemoveAll(dir)).ToNot(HaveOccurred())
}

func compileAuroraboot(targetPath string) {
	testDir, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred())

	parentDir := path.Join(testDir, "..")
	rootDir, err := filepath.Abs(parentDir)
	Expect(err).ToNot(HaveOccurred())

	cmd := exec.Command("go", "build", "-o", targetPath)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Dir = rootDir

	out, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), string(out))
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
