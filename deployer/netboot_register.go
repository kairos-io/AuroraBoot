package deployer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/pkg/netboot"
	"github.com/kairos-io/AuroraBoot/pkg/ops"
	"github.com/rs/zerolog/log"

	"github.com/spectrocloud-labs/herd"
)

// converts chema to Operations
// Netboot or not netboot
// If Not netboot:
//     download iso -> Edit it to attach cloud config (?) -> Offer link to download modified ISO with cloud config
//     download IPXE iso -> offer ISO that boots over ipxe with pixiecore
// TODO ops: start HTTP server, offer artifacts from dir
// TODO ops: download ISO save it to dir
//
//    or, offer generic IPXE iso -> and start netboot anyway

const (
	opDownloadInitrd   = "download-initrd"
	opDownloadKernel   = "download-kernel"
	opDownloadSquashFS = "download-squashfs"
	opPrepareNetboot   = "prepare-netboot"
	opStartNetboot     = "start-netboot"
)

func RegisterNetbootOperations(g *herd.Graph, artifact ReleaseArtifact, c Config, cloudConfigFile string) error {

	dst := c.StateDir("netboot")

	g.Add(opPrepareNetboot, herd.WithCallback(
		func(ctx context.Context) error {
			return os.MkdirAll(dst, 0700)
		},
	))

	g.Add(opDownloadInitrd, herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(artifact.InitrdURL(), dst)))
	g.Add(opDownloadKernel, herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(artifact.KernelURL(), dst)))
	g.Add(opDownloadSquashFS, herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(artifact.SquashFSURL(), dst)))

	//TODO: add Validate step
	g.Add(
		opStartNetboot,
		herd.WithDeps(opDownloadInitrd, opDownloadKernel, opDownloadSquashFS),
		herd.Background, // TODO: the dag should wait for background processes before returning from Run()
		herd.WithCallback(func(ctx context.Context) error {
			log.Info().Msgf("Start pixiecore")

			p, err := urlBase(artifact.SquashFSURL())
			if err != nil {
				return err
			}
			squashFSfile := filepath.Join(dst, p)

			p, err = urlBase(artifact.InitrdURL())
			if err != nil {
				return err
			}
			initrdFile := filepath.Join(dst, p)

			p, err = urlBase(artifact.KernelURL())
			if err != nil {
				return err
			}
			kernelFile := filepath.Join(dst, p)
			configFile := cloudConfigFile

			return netboot.Server(kernelFile, "AuroraBoot", fmt.Sprintf(`rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID "%s" }} netboot nodepair.enable config_url={{ ID "%s" }} console=tty1 console=ttyS0 console=tty0`, squashFSfile, configFile), []string{initrdFile})
		},
		),
	)

	return nil
}
