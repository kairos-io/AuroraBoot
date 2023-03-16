package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/rs/zerolog/log"
)

// TODO
func GenArmImages(src, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		log.Info().Msgf("Generating raw disk '%s' from '%s' to '%s'", dst, src)
		out, err := utils.SH(fmt.Sprintf("/prepare-arm-images.sh %s %s %s", src, dst, filepath.Join(dst, "config.yaml")))
		log.Printf("Output '%s'", out)
		if err != nil {
			log.Error().Msgf("Generating raw disk '%s' from '%s' to '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

// TODO
func GenArmDisk(src, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		log.Info().Msgf("Generating raw disk '%s' from '%s' to '%s'", dst, src)
		out, err := utils.SH(fmt.Sprintf("/build-arm-image.sh %s %s %s", src, dst, filepath.Join(dst, "config.yaml")))
		log.Printf("Output '%s'", out)
		if err != nil {
			log.Error().Msgf("Generating raw disk '%s' from '%s' to '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

func GenRawDisk(src, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		log.Info().Msgf("Generating raw disk '%s' from '%s'", dst, src)
		out, err := utils.SH(fmt.Sprintf("/raw-images.sh %s %s %s", src, dst, filepath.Join(filepath.Dir(dst), "config.yaml")))
		log.Printf("Output '%s'", out)
		if err != nil {
			log.Error().Msgf("Generating raw disk '%s' from '%s' to '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

func ExtractSquashFS(src, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		log.Info().Msgf("unpacking to '%s' the squashfs file: '%s'", dst, src)
		out, err := utils.SH(fmt.Sprintf("unsquashfs -f -d %s %s", dst, src))
		log.Printf("Output '%s'", out)
		if err != nil {
			log.Error().Msgf("unpacking to '%s' from '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

func ConvertRawDiskToVHD(src, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		log.Info().Msgf("Generating raw disk '%s' from '%s' to '%s'", dst, src)
		out, err := utils.SH(fmt.Sprintf("/azure.sh %s %s", src, dst))
		log.Printf("Output '%s'", out)
		if err != nil {
			log.Error().Msgf("Generating raw disk '%s' from '%s' to '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

func ConvertRawDiskToGCE(src, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		log.Info().Msgf("Generating raw disk '%s' from '%s' to '%s'", dst, src)
		out, err := utils.SH(fmt.Sprintf("/gce.sh %s %s", src, dst))
		log.Printf("Output '%s'", out)
		if err != nil {
			log.Error().Msgf("Generating raw disk '%s' from '%s' to '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}
