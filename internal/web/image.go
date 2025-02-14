package web

import (
	"os"
	"path/filepath"
)

func prepareDockerfile(tempdir string) error {
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
