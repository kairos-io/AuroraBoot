package ops

import (
	"context"
	"os"

	"github.com/kairos-io/AuroraBoot/internal"
	sdkUtils "github.com/kairos-io/kairos-sdk/utils"
)

// PullContainerImage pulls a container image either remotely or locally from a docker daemon.
func PullContainerImage(image, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		img, err := sdkUtils.GetImage(image, "", nil, nil)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("image", image).Msg("failed to pull image")
			return err
		}
		internal.Log.Logger.Info().Msgf("Pulling container image '%s' to '%s')", image, dst)

		// This method already first tries the local registry and then moves to remote, so no need to pass local
		err = os.MkdirAll(dst, os.ModeDir|os.ModePerm)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("image", image).Msg("failed to create directory")
		}
		err = sdkUtils.ExtractOCIImage(img, dst)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("image", image).Msg("failed to extract OCI image")
		}
		return err
	}
}
