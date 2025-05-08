package ops

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/constants"
	"github.com/kairos-io/AuroraBoot/pkg/netboot"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/kairos-sdk/iso"
)

// extractUKIArtifact extracts the UKI EFI file from the ISO
func extractUKIArtifact(src, dst, prefix string) error {
	artifact := filepath.Join(dst, fmt.Sprintf("%s.uki.efi", prefix))
	//err := iso.ExtractFileFromIso("/BOOTX64.EFI", src, artifact, &internal.Log)
	err := iso.ExtractFileFromIso("/norole.efi", src, artifact, &internal.Log)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artifact")
		return err
	}
	return nil
}

// extractTraditionalArtifacts extracts the traditional netboot artifacts (squashfs, kernel, initrd)
func extractTraditionalArtifacts(src, dst, prefix string) error {
	artifacts := map[string]string{
		"squashfs": "/rootfs.squashfs",
		"kernel":   "/boot/kernel",
		"initrd":   "/boot/initrd",
	}

	for artifactType, isoPath := range artifacts {
		artifactName := fmt.Sprintf("%s.%s", prefix, artifactType)
		if artifactType != "squashfs" {
			artifactName = fmt.Sprintf("%s-%s", prefix, artifactType)
		}

		artifact := filepath.Join(dst, artifactName)
		err := iso.ExtractFileFromIso(isoPath, src, artifact, &internal.Log)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("artifact", artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artifact")
			return err
		}
	}
	return nil
}

// ExtractNetboot extracts all the required netbooting artifacts
func ExtractNetboot(src, dst, prefix, netbootType string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		internal.Log.Logger.Info().Str("prefix", prefix).Str("source", src).Str("destination", dst).Msg("Extracting netboot artifacts")

		var err error
		if netbootType == "uki" {
			err = extractUKIArtifact(src, dst, prefix)
		} else {
			err = extractTraditionalArtifacts(src, dst, prefix)
		}

		if err != nil {
			return err
		}

		internal.Log.Logger.Info().Msg("Artifacts extracted")
		return nil
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

func StartPixiecoreUKI(address, netbootPort, ukiFile string, nb schema.NetBoot) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		internal.Log.Logger.Info().Msgf("Start UKI pixiecore")

		cmdLine := constants.UkiCmdline
		if nb.Cmdline != "" {
			cmdLine = constants.UkiCmdline + " " + nb.Cmdline
		}

		return netboot.ServerUKI(ukiFile, cmdLine, address, netbootPort, true)
	}
}
