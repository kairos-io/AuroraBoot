package e2e_test

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type Auroraboot struct {
	Path           string
	ContainerImage string
	Dirs           []string // directories to mount from host
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
		"--entrypoint", entrypoint,
		"-v", fmt.Sprintf("%s:/bin/auroraboot", e.Path),
	}

	for _, d := range e.Dirs {
		dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%[1]s:%[1]s", d))
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
