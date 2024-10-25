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
	enkitypes "github.com/kairos-io/enki/pkg/types"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
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

		// TODO: Are the args reveresed here? Copying from destination to source?
		// Or are we assuming StepCopyCloudConfig has already run, putting it the
		// config in "dst"? In that case, maybe move copying here, to a step?
		err = copy.Copy(filepath.Join(dst, "config.yaml"), filepath.Join(overlay, "config.yaml"))
		if err != nil {
			return err
		}

		internal.Log.Logger.Info().Msgf("Generating iso '%s' from '%s' to '%s'", name, src, dst)
		cfg := enkiconfig.NewBuildConfig(
			enkiconfig.WithLogger(sdkTypes.NewKairosLogger("enki", "debug", false)),
		)
		cfg.Name = name
		cfg.OutDir = dst
		// Live grub artifacts:
		// https://github.com/kairos-io/osbuilder/blob/95509370f6a87229879f1a381afa5d47225ce12d/tools-image/Dockerfile#L29-L30
		// but /efi is not needed because we handle it here:
		// https://github.com/kairos-io/enki/blob/6b92cbae96e92a1e36dfae2d5fdb5f3fb79bf99d/pkg/action/build-iso.go#L256
		// https://github.com/kairos-io/enki/blob/6b92cbae96e92a1e36dfae2d5fdb5f3fb79bf99d/pkg/action/build-iso.go#L325
		spec := &enkitypes.LiveISO{
			RootFS:             []*v1.ImageSource{v1.NewDirSrc(src)},
			Image:              []*v1.ImageSource{v1.NewDirSrc("/grub2"), v1.NewDirSrc(overlay)},
			Label:              "COS_LIVE",
			GrubEntry:          "Kairos",
			BootloaderInRootFs: false,
		}
		buildISO := enkiaction.NewBuildISOAction(cfg, spec)
		err = buildISO.ISORun()
		if err != nil {
			internal.Log.Logger.Error().Msgf("Failed generating iso '%s' from '%s'. Error: %s", name, src, err.Error())
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
