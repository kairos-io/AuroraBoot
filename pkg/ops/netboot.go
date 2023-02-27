package ops

import (
	"context"
	"fmt"

	"github.com/kairos-io/AuroraBoot/pkg/netboot"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
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

func StartPixiecore(cloudConfigFile, squashFSfile, address, netbootPort, initrdFile, kernelFile string, nb schema.NetBoot) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		log.Info().Msgf("Start pixiecore")

		configFile := cloudConfigFile

		cmdLine := `rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID "%s" }} netboot nodepair.enable config_url={{ ID "%s" }} console=tty1 console=ttyS0 console=tty0`

		if nb.Cmdline != "" {
			cmdLine = `root=live:{{ ID "%s" }} config_url={{ ID "%s" }} ` + nb.Cmdline
		}

		return netboot.Server(kernelFile, "AuroraBoot", fmt.Sprintf(cmdLine, squashFSfile, configFile), address, netbootPort, []string{initrdFile}, true)
	}
}
