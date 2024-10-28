package ops

import (
	"context"
	"fmt"

	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/netboot"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/kairos/pkg/utils"
)

// ExtractNetboot extracts all the required netbooting artifacts
func ExtractNetboot(src, dst, name string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		internal.Log.Logger.Info().Str("artifact", name).Str("source", src).Str("destination", dst).Msg("Extracting netboot artifacts")
		out, err := utils.SH(fmt.Sprintf("cd %s && /netboot.sh %s %s", dst, src, name))
		internal.Log.Logger.Printf("Output '%s'", out)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("artifact", name).Str("source", src).Str("destination", dst).Msg("Failed extracting netboot artfact")
		}
		return err
	}
}

func StartPixiecore(cloudConfigFile, squashFSfile, address, netbootPort, initrdFile, kernelFile string, nb schema.NetBoot) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		internal.Log.Logger.Info().Msgf("Start pixiecore")

		configFile := cloudConfigFile

		cmdLine := `rd.live.overlay.overlayfs rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID "%s" }} netboot nodepair.enable config_url={{ ID "%s" }} console=tty1 console=ttyS0 console=tty0`

		if nb.Cmdline != "" {
			cmdLine = `root=live:{{ ID "%s" }} config_url={{ ID "%s" }} ` + nb.Cmdline
		}

		return netboot.Server(kernelFile, fmt.Sprintf(cmdLine, squashFSfile, configFile), address, netbootPort, initrdFile, true)
	}
}
