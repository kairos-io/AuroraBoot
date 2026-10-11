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

// DefaultNetbootPrefix is the base name of the netboot artifacts when the
// configuration names none.
const DefaultNetbootPrefix = "kairos"

// NetbootArtifactPaths returns the three paths ExtractNetboot writes under dst
// for the given prefix, which is iso.name from the configuration.
//
// The server that serves those files has to ask for the same three names, so
// the producer and the consumer both read them from here. Spelling them twice
// is what made a run with iso.name set extract <name>-kernel and then serve
// kairos-kernel.
func NetbootArtifactPaths(dst, prefix string) (squashFS, kernel, initrd string) {
	if prefix == "" {
		prefix = DefaultNetbootPrefix
	}

	return filepath.Join(dst, fmt.Sprintf("%s.squashfs", prefix)),
		filepath.Join(dst, fmt.Sprintf("%s-kernel", prefix)),
		filepath.Join(dst, fmt.Sprintf("%s-initrd", prefix))
}

// ExtractNetboot extracts all the required netbooting artifacts
// isoFunc is a function that returns the path to the ISO file
// we need the function to be passed so its executed in the context of the deployer as otherwise
// the ISO file might not be available at the time of the call
func ExtractNetboot(isoFunc, dstFunc valueGetOnCall, prefix string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		src := isoFunc()
		dst := dstFunc()

		if _, err := os.Stat(dst); err != nil && os.IsNotExist(err) {
			return fmt.Errorf("destination directory %s does not exist: %w", dst, err)
		}
		internal.Log.Logger.Info().Str("prefix", prefix).Str("source", src).Str("destination", dst).Msg("Extracting netboot artifacts")

		squashFS, kernel, initrd := NetbootArtifactPaths(dst, prefix)

		for _, a := range []struct {
			inIso, artifact string
		}{
			{"/rootfs.squashfs", squashFS},
			{"/boot/kernel", kernel},
			{"/boot/initrd", initrd},
		} {
			if err := iso.ExtractFileFromIso(a.inIso, src, a.artifact, &internal.Log); err != nil {
				internal.Log.Logger.Error().Err(err).Str("artifact", a.artifact).Str("source", src).Str("destination", dst).Msgf("Failed extracting netboot artfact")
				return err
			}
		}

		internal.Log.Logger.Info().Msg("Artifacts extracted")

		return nil
	}
}

func StartPixiecore(cloudConfigFile, address, netbootPort string, squashFSfileGet, initrdFileGet, kernelFileGet valueGetOnCall, nb schema.NetBoot) func(ctx context.Context) error {
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
		}

		return netboot.Server(kernelFile, fmt.Sprintf(cmdLine, squashFSfile, configFile), address, netbootPort, initrdFile, true)
	}
}
