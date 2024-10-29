package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/kairos/pkg/utils"
	"github.com/otiai10/copy"

	enkiaction "github.com/kairos-io/enki/pkg/action"
	enkiconfig "github.com/kairos-io/enki/pkg/config"
	enkiconstants "github.com/kairos-io/enki/pkg/constants"
	enkitypes "github.com/kairos-io/enki/pkg/types"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
)

// GenISO generates an ISO from a rootfs, and stores results in dst
func GenISO(src, dst string, i schema.ISO) func(ctx context.Context) error {
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

		// We are assuming StepCopyCloudConfig has already run, putting it the config in "dst"
		err = copy.Copy(filepath.Join(dst, "config.yaml"), filepath.Join(overlay, "config.yaml"))
		if err != nil {
			return err
		}

		internal.Log.Logger.Info().Msgf("Generating iso '%s' from '%s' to '%s'", i.Name, src, dst)
		cfg := enkiconfig.NewBuildConfig(
			enkiconfig.WithLogger(sdkTypes.NewKairosLogger("enki", "debug", false)),
		)
		cfg.Name = i.Name
		cfg.OutDir = dst
		cfg.Date = i.IncludeDate
		if i.Arch != "" {
			cfg.Arch = i.Arch
		}
		isoLabel := enkiconstants.ISOLabel
		if i.Label != "" {
			isoLabel = i.Label
		}
		spec := &enkitypes.LiveISO{
			RootFS:             []*v1.ImageSource{v1.NewDirSrc(src)},
			Image:              []*v1.ImageSource{v1.NewDirSrc("/grub2"), v1.NewDirSrc(overlay)},
			Label:              isoLabel,
			GrubEntry:          "Kairos",
			BootloaderInRootFs: false,
		}

		if i.OverlayRootfs != "" {
			spec.RootFS = append(spec.RootFS, v1.NewDirSrc(i.OverlayRootfs))
		}
		if i.OverlayUEFI != "" {
			// TODO: Doesn't seem to do anything on enki.
			spec.UEFI = append(spec.UEFI, v1.NewDirSrc(i.OverlayUEFI))
		}
		if i.OverlayISO != "" {
			spec.Image = append(spec.Image, v1.NewDirSrc(i.OverlayISO))
		}

		buildISO := enkiaction.NewBuildISOAction(cfg, spec)
		err = buildISO.ISORun()
		if err != nil {
			internal.Log.Logger.Error().Msgf("Failed generating iso '%s' from '%s'. Error: %s", i.Name, src, err.Error())
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
			internal.Log.Logger.Info().Msgf("Adding data in '%s' to '%s'", i.DataPath, isoFile)
			err = copy.Copy(i.DataPath, tmp)
			if err != nil {
				return err
			}
		}

		internal.Log.Logger.Info().Msgf("Adding cloud config file to '%s'", isoFile)
		err = copy.Copy(filepath.Join(dst, "config.yaml"), filepath.Join(tmp, "config.yaml"))
		if err != nil {
			return err
		}

		out, err := utils.SH(fmt.Sprintf("xorriso -indev %s -outdev %s -map %s / -boot_image any replay", isoFile, injectedIso, tmp))
		internal.Log.Logger.Print(out)
		if err != nil {
			return err
		}
		internal.Log.Logger.Info().Msgf("Wrote '%s'", injectedIso)
		return err
	}
}
