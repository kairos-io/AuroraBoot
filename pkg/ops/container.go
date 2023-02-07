package ops

import (
	"context"
	"fmt"

	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/rs/zerolog/log"
)

// PullContainerImage pulls a container image either remotely or locally from a docker daemon.
func PullContainerImage(image, dst string, local bool) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		log.Info().Msgf("Pulling container image '%s' to '%s' (local: %t)", image, dst, local)
		l := ""
		if local {
			l = "--local"
		}
		out, err := utils.SH(fmt.Sprintf("luet util unpack %s %s %s", l, image, dst))
		log.Printf("Output '%s'", out)
		if err != nil {
			log.Error().Msgf("Failed pulling container image '%s' to '%s' (local: %t): %s", image, dst, local, err.Error())
		}
		return err
	}
}
