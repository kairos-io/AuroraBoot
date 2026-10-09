package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/internal"
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
		internal.Log.Logger.Info().Msg("Artifacts extracted")

		return err
	}
}

func StartPixiecore(cloudConfigFile, address, netbootPort string, squashFSfileGet, initrdFileGet, kernelFileGet valueGetOnCall, nb schema.NetBoot) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		internal.Log.Logger.Info().Msgf("Start pixiecore")
		// do them in the context of the deployer so we can use the functions at the time of the call
		squashFSfile := squashFSfileGet()
		initrdFile := initrdFileGet()
		kernelFile := kernelFileGet()

		cmdLine := netbootCmdline(squashFSfile, cloudConfigFile, nb)
		internal.Log.Logger.Info().Str("cmdline", cmdLine).Msg("Netbooting")

		return netboot.Server(kernelFile, cmdLine, address, netbootPort, initrdFile, true)
	}
}

// netbootCmdline builds the cmdline a netbooted node boots with.
//
// The default carries install-mode because a netboot is an install. The
// keyword is what makes 52_installer.yaml write kairos-installer.service, and
// that unit is the only thing that runs the install: kairos-agent start stands
// aside on a netboot when the config asks for an unattended install, and the
// pairing code a node with no config has to show comes from the same unit. The
// rest of that guard already holds on a netboot, because a cmdline carrying
// netboot is a LiveCD boot to kairos-sdk and immucore writes
// /run/cos/live_mode for it. See kairos-io/kairos#5373.
//
// An explicit netboot.cmdline replaces the whole default, keeping only the two
// keys netboot has to control, so an operator who wants a live node that does
// not install still has a way to ask for one.
func netbootCmdline(squashFSfile, configFile string, nb schema.NetBoot) string {
	cmdLine := `rd.live.overlay.overlayfs rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID "%s" }} netboot install-mode nodepair.enable config_url={{ ID "%s" }} console=tty1 console=ttyS0 console=tty0`

	if nb.Cmdline != "" {
		cmdLine = `root=live:{{ ID "%s" }} config_url={{ ID "%s" }} ` + nb.Cmdline
	}

	return fmt.Sprintf(cmdLine, squashFSfile, configFile)
}
