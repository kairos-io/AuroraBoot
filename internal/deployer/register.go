package deployer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spectrocloud-labs/herd"
)

// converts chema to Operations
// Netboot or not netboot
// If Not netboot:
//     download iso -> Edit it to attach cloud config (?) -> Offer link to download modified ISO with cloud config
//     download IPXE iso -> offer ISO that boots over ipxe with pixiecore
//
//    or, offer generic IPXE iso -> and start netboot anyway

const (
	opDownloadInitrd   = "download-initrd"
	opDownloadKernel   = "download-kernel"
	opDownloadSquashFS = "download-squashfs"

	opStartNetboot = "start-netboot"
)

func sh(command, dir string) error {
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = dir
	return cmd.Run()
}

func RegisterOperations(g *herd.Graph, artifact ReleaseArtifact) error {

	dst := "/tmp/netboot"
	os.MkdirAll("/tmp/netboot", 0700)

	g.Add(opDownloadInitrd, herd.WithCallback(dowloadArtifact(artifact.InitrdURL(), dst)))
	g.Add(opDownloadKernel, herd.WithCallback(dowloadArtifact(artifact.KernelURL(), dst)))
	g.Add(opDownloadSquashFS, herd.WithCallback(dowloadArtifact(artifact.SquashFSURL(), dst)))

	//TODO: add Validate step
	g.Add(
		opStartNetboot,
		herd.WithDeps(opDownloadInitrd, opDownloadKernel, opDownloadSquashFS),
		herd.WithCallback(func(ctx context.Context) error {
			fmt.Println("Start pixiecore")

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
			configFile := ""
			return sh(fmt.Sprintf(`pixiecore boot %s %s --cmdline="rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID \"%s\" }} netboot nodepair.enable config_url={{ ID \"%s\" }} console=tty1 console=ttyS0 console=tty0"`, kernelFile, initrdFile, squashFSfile, configFile), "/tmp/netboot")
		},
		),
	)

	return nil
}
