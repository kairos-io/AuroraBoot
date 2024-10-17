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

	enkiaction "github.com/kairos-io/enki/pkg/action"
	enkiconfig "github.com/kairos-io/enki/pkg/config"
	enkitypes "github.com/kairos-io/enki/pkg/types"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
	"github.com/spf13/pflag"
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
		cfg, err := enkiconfig.ReadConfigBuild("/config", &pflag.FlagSet{})
		if err != nil {
			return err
		}
		cfg.Name = name
		cfg.OutDir = dst
		cfg.Logger = sdkTypes.NewKairosLogger("enki", "debug", false)
		//logrus.SetLevel(logrus.DebugLevel) // what on earth? Globally and for all deps?
		spec := &enkitypes.LiveISO{}
		imgSource, err := v1.NewSrcFromURI("dir:" + src)
		if err != nil {
			cfg.Logger.Errorf("not a valid rootfs source: %s", src)
			return err
		}
		spec.RootFS = []*v1.ImageSource{imgSource}

		uefi := v1.NewDirSrc("/efi")
		grub := v1.NewDirSrc("/grub2")
		isoOverlay := v1.NewDirSrc(overlay)
		spec.UEFI = append(spec.UEFI, uefi)
		spec.Image = append(spec.Image, uefi, grub, isoOverlay)
		spec.Label = "COS_LIVE"
		spec.GrubEntry = "Kairos"
		spec.BootloaderInRootFs = false

		buildISO := enkiaction.NewBuildISOAction(cfg, spec)
		err = buildISO.ISORun()
		if err != nil {
			cfg.Logger.Errorf(err.Error())
		}
		return err

		// out, err := utils.SH(fmt.Sprintf("/entrypoint.sh --debug --name %s build-iso --squash-no-compression --overlay-iso %s --date=false --output %s dir:%s", name, overlay, dst, src))
		// fmt.Printf("out = %+v\n", out)
		// log.Printf("Output '%s'", out)
		// if err != nil {
		// 	log.Error().Msgf("Failed generating iso '%s' from '%s'. Error: %s", name, src, err.Error())
		// }
		// return err
	}
}

func InjectISO(dst, isoFile string, i schema.ISO) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		os.Chdir(dst)
		injectedIso := isoFile + ".custom.iso"
		os.Remove(injectedIso)

		tmp, err := os.MkdirTemp("", "injectiso")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		if i.DataPath != "" {
			log.Info().Msgf("Adding data in '%s' to '%s'", i.DataPath, isoFile)
			err = copy.Copy(i.DataPath, tmp)
			if err != nil {
				return err
			}
		}

		log.Info().Msgf("Adding cloud config file to '%s'", isoFile)
		err = copy.Copy(filepath.Join(dst, "config.yaml"), filepath.Join(tmp, "config.yaml"))
		if err != nil {
			return err
		}

		out, err := utils.SH(fmt.Sprintf("xorriso -indev %s -outdev %s -map %s / -boot_image any replay", isoFile, injectedIso, tmp))
		log.Print(out)
		if err != nil {
			return err
		}
		log.Info().Msgf("Wrote '%s'", injectedIso)
		return err
	}
}
