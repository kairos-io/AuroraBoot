package web

import (
	"fmt"
	"os"
	"path/filepath"
)

func dockerCommand(
	contextDir, image string,
	karosInitVersion string,
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
	--build-arg KAIROS_INIT=%s \
	-t %s`, contextDir, variant, model, trustedBoot, kubernetesProvider, kubernetesVersion, karosInitVersion, image)
}

func prepareImage(tempdir string) error {
	// Copy the Dockerfile
	dockerFile, err := assets.ReadFile("assets/Dockerfile")
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(tempdir, "Dockerfile"), dockerFile, 0644)
	if err != nil {
		return err
	}

	return nil
}
