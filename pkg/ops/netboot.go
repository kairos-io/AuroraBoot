package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/constants"
	"github.com/kairos-io/AuroraBoot/pkg/netboot"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/kairos/v4/sdk/iso"
)

// valueGetOnCall is a function type that returns a string value.
// It is used to defer the retrieval of a value until the function is called,
// allowing for dynamic values to be fetched at the time of the call.
// This is mainly due to the fact that the deployer might not have the values available at the time of the registration,
// but rather at the time of the call, e.g. when the ISO file is downloaded or the destination directory is created.
type valueGetOnCall func() string

// ExtractNetboot extracts all the required netbooting artifacts
// isoFunc is a function that returns the path to the ISO file
// we need the function to be passed so its executed in the context of the deployer as otherwise
// the ISO file might not be available at the time of the call
func ExtractNetboot(isoFunc, dstFunc valueGetOnCall, prefix string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		src := isoFunc()
		dst := dstFunc()

		if prefix == "" {
			prefix = "kairos"
		}

		if _, err := os.Stat(dst); err != nil && os.IsNotExist(err) {
			return fmt.Errorf("destination directory %s does not exist: %w", dst, err)
		}
		internal.Log.Logger.Info().Str("prefix", prefix).Str("source", src).Str("destination", dst).Msg("Extracting netboot artifacts")

		artifact := filepath.Join(dst, fmt.Sprintf("%s.squashfs", prefix))
		err := iso.ExtractFileFromIso("/rootfs.squashfs", src, artifact, &internal.Log)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artfact")
			return err
		}
		artifact = filepath.Join(dst, fmt.Sprintf("%s-kernel", prefix))
		err = iso.ExtractFileFromIso("/boot/kernel", src, artifact, &internal.Log)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artfact")
			return err
		}
		artifact = filepath.Join(dst, fmt.Sprintf("%s-initrd", prefix))
		err = iso.ExtractFileFromIso("/boot/initrd", src, artifact, &internal.Log)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artfact")
			return err
		}

		// The livecd grub config holds the cmdline the ISO boots with, which
		// the netboot cmdline is meant to match. Not every ISO has one, so a
		// miss only costs the caller those options: hence the nil logger, the
		// error of an expected miss does not belong on the Error channel.
		artifact = filepath.Join(dst, fmt.Sprintf("%s-grub.cfg", prefix))
		if err := iso.ExtractFileFromIso(filepath.Join(constants.GrubPrefixDir, constants.GrubCfg), src, artifact, nil); err != nil {
			internal.Log.Logger.Warn().Err(err).Str("artifact", artifact).Str("source", src).Msg("No livecd grub config in the ISO, netboot will boot with the default cmdline")
			_ = os.Remove(artifact)
		}

		internal.Log.Logger.Info().Msg("Artifacts extracted")

		return nil
	}
}

func StartPixiecore(cloudConfigFile, address, netbootPort string, squashFSfileGet, initrdFileGet, kernelFileGet, grubCfgFileGet valueGetOnCall, nb schema.NetBoot) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		internal.Log.Logger.Info().Msgf("Start pixiecore")
		// do them in the context of the deployer so we can use the functions at the time of the call
		squashFSfile := squashFSfileGet()
		initrdFile := initrdFileGet()
		kernelFile := kernelFileGet()

		configFile := cloudConfigFile

		cmdLine := `rd.live.overlay.overlayfs rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID "%s" }} netboot nodepair.enable config_url={{ ID "%s" }} console=tty1 console=ttyS0 console=tty0`

		if nb.Cmdline != "" {
			cmdLine = `root=live:{{ ID "%s" }} config_url={{ ID "%s" }} ` + nb.Cmdline
		} else {
			// Without an explicit override, take the options the ISO itself
			// boots with instead of leaving this list to drift away from it.
			// See kairos-io/kairos#2573.
			cmdLine = withLiveCmdline(cmdLine, grubCfgFileGet)
		}

		cmdLine = fmt.Sprintf(cmdLine, squashFSfile, configFile)
		internal.Log.Logger.Info().Str("cmdline", cmdLine).Msg("Netbooting")

		return netboot.Server(kernelFile, cmdLine, address, netbootPort, initrdFile, true)
	}
}

// withLiveCmdline merges the livecd grub config extracted by ExtractNetboot
// into the netboot cmdline. A grub config that is absent or unreadable is not
// an error: the caller may be netbooting an ISO that has none, and the cmdline
// is then used as it is.
func withLiveCmdline(cmdLine string, grubCfgFileGet valueGetOnCall) string {
	if grubCfgFileGet == nil {
		return cmdLine
	}

	grubCfgFile := grubCfgFileGet()
	if grubCfgFile == "" {
		return cmdLine
	}

	grubCfg, err := os.ReadFile(grubCfgFile)
	if err != nil {
		internal.Log.Logger.Warn().Err(err).Str("grubCfg", grubCfgFile).Msg("Could not read the livecd grub config, netbooting with the default cmdline")
		return cmdLine
	}

	merged := netboot.LiveCmdline(cmdLine, string(grubCfg))
	internal.Log.Logger.Debug().Str("grubCfg", grubCfgFile).Msg("Netboot cmdline taken from the livecd grub config")

	return merged
}
