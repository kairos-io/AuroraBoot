package ops

import (
	"context"
	"fmt"

	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/rs/zerolog/log"
)

// GenISO generates an ISO from a rootfs, and stores results in dst
func GenISO(name, src, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		log.Info().Msgf("Generating iso '%s' from '%s' to '%s'", name, src, dst)
		out, err := utils.SH(fmt.Sprintf("/entrypoint.sh --debug --name %s build-iso --date=false --output %s dir:%s", name, dst, src))
		log.Printf("Output '%s'", out)
		if err != nil {
			log.Error().Msgf("Failed generating iso '%s' from '%s'. Error: %s", name, src, err.Error())
		}
		return err
	}
}
