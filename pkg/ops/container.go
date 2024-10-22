package ops

import (
	"context"

	sdkUtils "github.com/kairos-io/kairos-sdk/utils"
	"github.com/rs/zerolog/log"
)

// PullContainerImage pulls a container image either remotely or locally from a docker daemon.
func PullContainerImage(image, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		img, err := sdkUtils.GetImage(image, "", nil, nil)
		if err != nil {
			log.Error().Err(err).Str("image", image).Msg("failed to pull image")
			return err
		}
		log.Info().Msgf("Pulling container image '%s' to '%s')", image, dst)
		// This method already first tries the local registry and then moves to remote, so no need to pass local
		err = sdkUtils.ExtractOCIImage(img, dst)
		if err != nil {
			log.Error().Err(err).Str("image", image).Msg("failed to extract OCI image")
		}
		return err
	}
}
