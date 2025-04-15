package ops

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/internal/log"
	"github.com/kairos-io/AuroraBoot/pkg/netboot"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/kairos-sdk/iso"
)

// ExtractNetboot extracts all the required netbooting artifacts
func ExtractNetboot(src, dst, prefix string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		log.Log.Logger.Info().Str("prefix", prefix).Str("source", src).Str("destination", dst).Msg("Extracting netboot artifacts")

		artifact := filepath.Join(dst, fmt.Sprintf("%s.squashfs", prefix))
		err := iso.ExtractFileFromIso("/rootfs.squashfs", src, artifact, &log.Log)
		if err != nil {
			log.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artfact")
			return err
		}
		artifact = filepath.Join(dst, fmt.Sprintf("%s-kernel", prefix))
		err = iso.ExtractFileFromIso("/boot/kernel", src, artifact, &log.Log)
		if err != nil {
			log.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artfact")
			return err
		}
		artifact = filepath.Join(dst, fmt.Sprintf("%s-initrd", prefix))
		err = iso.ExtractFileFromIso("/boot/initrd", src, artifact, &log.Log)
		if err != nil {
			log.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artfact")
			return err
		}
		log.Log.Logger.Info().Msg("Artifacts extracted")

		return err
	}
}

func StartPixiecore(cloudConfigFile, squashFSfile, address, netbootPort, initrdFile, kernelFile string, nb schema.NetBoot) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		log.Log.Logger.Info().Msgf("Start pixiecore")

		configFile := cloudConfigFile

		cmdLine := `rd.live.overlay.overlayfs rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID "%s" }} netboot nodepair.enable config_url={{ ID "%s" }} console=tty1 console=ttyS0 console=tty0`

		if nb.Cmdline != "" {
			cmdLine = `root=live:{{ ID "%s" }} config_url={{ ID "%s" }} ` + nb.Cmdline
		}

		return netboot.Server(kernelFile, fmt.Sprintf(cmdLine, squashFSfile, configFile), address, netbootPort, initrdFile, true)
	}
}
