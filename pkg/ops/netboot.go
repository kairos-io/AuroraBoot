package ops

import (
	"context"
	"fmt"

	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/rs/zerolog/log"
)

// ExtractNetboot extracts all the required netbooting artifacts
func ExtractNetboot(src, dst, name string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		log.Info().Msgf("Extracting netboot artifacts '%s' from '%s' to '%s'", name, src, dst)
		out, err := utils.SH(fmt.Sprintf("cd %s && /netboot.sh %s %s", dst, src, name))
		log.Printf("Output '%s'", out)
		if err != nil {
			log.Error().Msgf("Failed extracting netboot artfact '%s' from '%s'. Error: %s", name, src, err.Error())
		}
		return err
	}
}
