package deployer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kairos-io/AuroraBoot/pkg/ops"
	"github.com/rs/zerolog/log"

	"github.com/spectrocloud-labs/herd"
	"go.universe.tf/netboot/out/ipxe"
	"go.universe.tf/netboot/pixiecore"
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

	opStartNetboot = "start-netboot"
)

func pixieCore(kernel, bootmsg, cmdline string, initrds []string) error {

	spec := &pixiecore.Spec{
		Kernel:  pixiecore.ID(kernel),
		Cmdline: cmdline,
		Message: bootmsg,
	}
	for _, initrd := range initrds {
		spec.Initrd = append(spec.Initrd, pixiecore.ID(initrd))
	}

	booter, err := pixiecore.StaticBooter(spec)
	if err != nil {
		return err
	}

	ipxeFw := map[pixiecore.Firmware][]byte{}
	ipxeFw[pixiecore.FirmwareX86PC] = ipxe.MustAsset("third_party/ipxe/src/bin/undionly.kpxe")
	ipxeFw[pixiecore.FirmwareEFI32] = ipxe.MustAsset("third_party/ipxe/src/bin-i386-efi/ipxe.efi")
	ipxeFw[pixiecore.FirmwareEFI64] = ipxe.MustAsset("third_party/ipxe/src/bin-x86_64-efi/ipxe.efi")
	ipxeFw[pixiecore.FirmwareEFIBC] = ipxe.MustAsset("third_party/ipxe/src/bin-x86_64-efi/ipxe.efi")
	ipxeFw[pixiecore.FirmwareX86Ipxe] = ipxe.MustAsset("third_party/ipxe/src/bin/ipxe.pxe")
	s := &pixiecore.Server{
		Ipxe:           ipxeFw,
		Log:            func(subsystem, msg string) { log.Printf("%s: %s\n", subsystem, msg) },
		HTTPPort:       80,
		HTTPStatusPort: 0,
		DHCPNoBind:     false,
		Address:        "0.0.0.0",
		UIAssetsDir:    "",
	}
	s.Booter = booter

	return s.Serve()
}

func RegisterNetbootOperations(g *herd.Graph, artifact ReleaseArtifact, cloudConfigFile string) error {

	dst := "/tmp/netboot"
	os.MkdirAll("/tmp/netboot", 0700)

	g.Add(opDownloadInitrd, herd.WithCallback(ops.DownloadArtifact(artifact.InitrdURL(), dst)))
	g.Add(opDownloadKernel, herd.WithCallback(ops.DownloadArtifact(artifact.KernelURL(), dst)))
	g.Add(opDownloadSquashFS, herd.WithCallback(ops.DownloadArtifact(artifact.SquashFSURL(), dst)))

	//TODO: add Validate step
	g.Add(
		opStartNetboot,
		herd.WithDeps(opDownloadInitrd, opDownloadKernel, opDownloadSquashFS),
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

			return pixieCore(kernelFile, "AuroraBoot", fmt.Sprintf(`rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID "%s" }} netboot nodepair.enable config_url={{ ID "%s" }} console=tty1 console=ttyS0 console=tty0`, squashFSfile, configFile), []string{initrdFile})
		},
		),
	)

	return nil
}
