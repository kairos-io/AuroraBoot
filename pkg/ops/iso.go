package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/otiai10/copy"
	"github.com/rs/zerolog/log"
)

// GenISO generates an ISO from a rootfs, and stores results in dst
func GenISO(name, src, dst string, i schema.ISO) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "geniso")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		overlay := tmp
		if i.DataPath != "" {
			overlay = i.DataPath
		}

		err = copy.Copy(filepath.Join(dst, "config.yaml"), filepath.Join(overlay, "config.yaml"))
		if err != nil {
			return err
		}

		log.Info().Msgf("Generating iso '%s' from '%s' to '%s'", name, src, dst)
		out, err := utils.SH(fmt.Sprintf("/entrypoint.sh --debug --name %s build-iso --overlay-iso %s --date=false --output %s dir:%s", name, overlay, dst, src))
		log.Printf("Output '%s'", out)
		if err != nil {
			log.Error().Msgf("Failed generating iso '%s' from '%s'. Error: %s", name, src, err.Error())
		}
		return err
	}
}

func InjectISO(dst, isoFile string, i schema.ISO) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		os.Chdir(dst)
		injectedIso := isoFile + ".custom.iso"
		os.Remove(injectedIso)

		log.Info().Msgf("Adding cloud config file to '%s'", isoFile)

		tmp, err := os.MkdirTemp("", "injectiso")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		err = copy.Copy(i.DataPath, tmp)
		if err != nil {
			return err
		}

		err = copy.Copy(filepath.Join(dst, "config.yaml"), filepath.Join(tmp, "config.yaml"))
		if err != nil {
			return err
		}

		out, err := utils.SH(fmt.Sprintf("xorriso -indev %s -outdev %s -map %s / -boot_image any replay", isoFile, injectedIso, tmp))
		log.Print(out)
		if err != nil {
			return err
		}

		return err
	}
}
