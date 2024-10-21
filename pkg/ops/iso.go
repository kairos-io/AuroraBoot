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
		imgSource := v1.NewDirSrc(src)
		grub := v1.NewDirSrc("/grub2")
		isoOverlay := v1.NewDirSrc(overlay)

		// Removed here: https://github.com/kairos-io/osbuilder/commit/866dc42c48878fa8bebab26d8f24bf0f2953eb6e#diff-5e18c6520d9fb094d1d4c52b4fb652f09becd65053d95f8d7ac89c7998172fd4
		// enki is called here: https://github.com/kairos-io/AuroraBoot/blob/9ca30fc6e3b6cac2d227e7937bd6895198bdc4d0/pkg/ops/iso.go#L34
		// config is loaded here: https://github.com/kairos-io/enki/blob/844bd15261e89f103c16d1caa656ec41524b8d4f/pkg/config/config.go#L110-L114
		// config was copied in place here: https://github.com/kairos-io/osbuilder/blob/405eda716a6c291eb539a98765a291e893d0eda1/tools-image/Dockerfile#L86
		// which is part of the auroraboot image: https://github.com/kairos-io/AuroraBoot/blob/9ca30fc6e3b6cac2d227e7937bd6895198bdc4d0/Dockerfile#L1
		// TODO: Cleanup this madness!

		//uefi := v1.NewDirSrc("/efi")
		//spec.UEFI = append(spec.UEFI, uefi)
		//spec.Image = append(spec.Image, uefi, grub, isoOverlay)
		spec := &enkitypes.LiveISO{
			RootFS:             []*v1.ImageSource{imgSource},
			Image:              []*v1.ImageSource{grub, isoOverlay},
			Label:              "COS_LIVE",
			GrubEntry:          "Kairos",
			BootloaderInRootFs: false,
		}
		buildISO := enkiaction.NewBuildISOAction(cfg, spec)
		err = buildISO.ISORun()
		if err != nil {
			cfg.Logger.Errorf(err.Error())
		}
		return err
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
