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
) string {
	return fmt.Sprintf(`docker build %s -t %s`, contextDir, image)
}

func saveOCI(dst, image string) string {
	return fmt.Sprintf("docker save -o %s %s", dst, image)
}

func runBashProcessWithOutput(ws io.Writer, command string) error {
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdout = ws
	cmd.Stderr = ws
	return cmd.Run()
}
