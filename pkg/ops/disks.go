package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/internal/log"
	"github.com/kairos-io/kairos-sdk/utils"
)

func GenEFIRawDisk(src, dst string, size uint64) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		log.Log.Logger.Info().Msgf("Generating raw disk '%s' from '%s' with final size %dMb", dst, src, size)
		// TODO: We need to talk about how the config.yaml is magically here no? is done in a previous step but maybe we should have constant that we can check?
		// Maybe on its own function that returns the tmpdir + config.yaml or something? we need a safe way of accessing it form any step in the DAG.
		raw := NewEFIRawImage(src, dst, filepath.Join(dst, "config.yaml"), size)
		err := raw.Build()
		if err != nil {
			log.Log.Logger.Error().Msgf("Generating raw disk '%s' from '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

func GenBiosRawDisk(src, dst string, size uint64) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		log.Log.Logger.Info().Msgf("Generating raw disk '%s' from '%s' with final size %dMb", dst, src, size)
		// TODO: We need to talk about how the config.yaml is magically here no? is done in a previous step but maybe we should have constant that we can check?
		// Maybe on its own function that returns the tmpdir + config.yaml or something? we need a safe way of accessing it form any step in the DAG.
		raw := NewBiosRawImage(src, dst, filepath.Join(dst, "config.yaml"), size)
		err := raw.Build()
		if err != nil {
			log.Log.Logger.Error().Msgf("Generating raw disk '%s' from '%s' failed with error '%s'", dst, src, err.Error())
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

		log.Log.Logger.Info().Msgf("unpacking to '%s' the squashfs file: '%s'", dst, src)
		out, err := utils.SH(fmt.Sprintf("unsquashfs -f -d %s %s", dst, src))
		log.Log.Logger.Printf("Output '%s'", out)
		if err != nil {
			log.Log.Logger.Error().Msgf("unpacking to '%s' from '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

func ConvertRawDiskToVHD(src string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		glob, err := filepath.Glob(filepath.Join(src, "kairos-*.raw"))
		if err != nil {
			return err
		}

		if len(glob) == 0 || len(glob) > 1 {
			return fmt.Errorf("expected to find one and only one raw disk file in '%s' but found %d", src, len(glob))
		}

		log.Log.Logger.Info().Msgf("Generating raw disk from '%s'", glob[0])
		output, err := Raw2Azure(glob[0])
		if err != nil {
			log.Log.Logger.Error().Msgf("Generating raw disk from '%s' failed with error '%s'", glob[0], err.Error())
		} else {
			log.Log.Logger.Info().Msgf("Generated VHD disk '%s'", output)
		}
		return err
	}
}

func ConvertRawDiskToGCE(src string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		glob, err := filepath.Glob(filepath.Join(src, "kairos-*.raw"))
		if err != nil {
			return err
		}

		if len(glob) == 0 || len(glob) > 1 {
			return fmt.Errorf("expected to find one and only one raw disk file in '%s' but found %d", src, len(glob))
		}

		log.Log.Logger.Info().Msgf("Generating raw disk '%s'", glob[0])
		output, err := Raw2Gce(glob[0])
		if err != nil {
			log.Log.Logger.Error().Msgf("Generating raw disk from '%s' failed with error '%s'", src, err.Error())
		} else {
			log.Log.Logger.Info().Msgf("Generated GCE disk '%s'", output)
		}
		return err
	}
}
