package web

import (
	"fmt"
	"io"
	"os/exec"
)

func buildRawDisk(containerImage, outputDir string) string {
	return fmt.Sprintf(`auroraboot \
	--debug  \
	--set "disable_http_server=true" \
	--set "disable_netboot=true" \
	--set "container_image=%s" \
	--set "state_dir=%s" \
	--set "disk.raw=true" \
	`, containerImage, outputDir)
}

func buildISO(containerImage, outputDir, artifactName string) string {
	return fmt.Sprintf(`auroraboot --debug build-iso \
	--output %s \
	--name %s \
	docker:%s`, outputDir, artifactName, containerImage)
}

func buildOCI(
	contextDir,
	image string,
	karosInitVersion string,
	baseImage string,
	variant string,
	model string,
	trustedBoot bool,
	kubernetesProvider string,
	kubernetesVersion string,
) string {
	return fmt.Sprintf(`docker build %s \
	--build-arg VARIANT=%s \
	--build-arg MODEL=%s \
	--build-arg TRUSTED_BOOT=%t \
	--build-arg KUBERNETES_PROVIDER=%s \
	--build-arg KUBERNETES_VERSION=%s \
	--build-arg KAIROS_INIT_VERSION=%s \
	--build-arg BASE_IMAGE=%s \
	-t %s`, contextDir, variant, model, trustedBoot, kubernetesProvider, kubernetesVersion, karosInitVersion, baseImage, image)
}

func saveOCI(dst, image string) string {
	return fmt.Sprintf("docker save -o %s %s", dst, image)
}

func runBashProcessWithOutput(ws io.Writer, command string) error {
	// Simulate a background process
	cmd := exec.Command("bash", "-c", command)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	out := io.MultiReader(stdout, stderr)

	if err := cmd.Start(); err != nil {
		return err
	}

	// Stream process output to writer
	reader := io.TeeReader(ansiToHTML(out), ws)
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return err
	}

	if err := cmd.Wait(); err != nil {
		return err
	}

	return nil
}
