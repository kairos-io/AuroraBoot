package deployer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/kairos-io/AuroraBoot/pkg/netboot"
	"github.com/kairos-io/AuroraBoot/pkg/ops"
	"github.com/kairos-io/kairos/pkg/utils"

	"github.com/spectrocloud-labs/herd"
)

const (
	opDownloadISO     = "download-iso"
	opCopyCloudConfig = "copy-cloud-config"
	opPrepareISO      = "prepare-iso"
	opStartHTTPServer = "start-httpserver"
	opInjectCC        = "inject-cloud-config"

	opDownloadInitrd   = "download-initrd"
	opDownloadKernel   = "download-kernel"
	opDownloadSquashFS = "download-squashfs"
	opPrepareNetboot   = "prepare-netboot"
	opStartNetboot     = "start-netboot"

	opContainerPull  = "container-pull"
	opGenISO         = "gen-iso"
	opPreparetmproot = "prepare-temp"
	opExtractNetboot = "extract-netboot"
)

const (
	kairosDefaultArtifactName = "kairos"
)

// Register register the op dag based on the configuration and the artifact wanted.
func Register(g *herd.Graph, artifact ReleaseArtifact, c Config, cloudConfigFile string) {
	dst := c.StateDir("iso")
	dstNetboot := c.StateDir("netboot")

	// squashfs, kernel, and initrd names are tied to the output of /netboot.sh (op.ExtractNetboot)
	squashFSfile := filepath.Join(dstNetboot, "kairos.squashfs")
	kernelFile := filepath.Join(dstNetboot, "kairos-kernel")
	initrdFile := filepath.Join(dstNetboot, "kairos-initrd")
	isoFile := filepath.Join(dst, "kairos.iso")

	tmpRootfs := c.StateDir("temp-rootfs")

	g.Add(opPreparetmproot, herd.WithCallback(
		func(ctx context.Context) error {
			return os.MkdirAll(dstNetboot, 0700)
		},
	))
	g.Add(opPrepareNetboot, herd.WithCallback(
		func(ctx context.Context) error {
			return os.MkdirAll(dstNetboot, 0700)
		},
	))

	g.Add(opPrepareISO, herd.WithCallback(func(ctx context.Context) error {
		return os.MkdirAll(dst, 0700)
	}))

	fromImage := artifact.ContainerImage != ""

	// Pull locak docker daemon if container image starts with docker://
	containerImage := artifact.ContainerImage
	local := false

	if strings.HasPrefix(containerImage, "docker://") {
		local = true
		containerImage = strings.ReplaceAll(containerImage, "docker://", "")
	}

	if fromImage {
		g.Add(opContainerPull, herd.WithDeps(opPreparetmproot), herd.WithCallback(ops.PullContainerImage(containerImage, tmpRootfs, local)))
		g.Add(opGenISO, herd.WithDeps(opContainerPull), herd.WithCallback(ops.GenISO(kairosDefaultArtifactName, tmpRootfs, dst)))
		g.Add(opExtractNetboot, herd.WithDeps(opGenISO), herd.WithCallback(ops.ExtractNetboot(isoFile, dstNetboot, kairosDefaultArtifactName)))
	} else {
		//TODO: add Validate step
		g.Add(opDownloadInitrd, herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(artifact.InitrdURL(), initrdFile)))
		g.Add(opDownloadKernel, herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(artifact.KernelURL(), kernelFile)))
		g.Add(opDownloadSquashFS, herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(artifact.SquashFSURL(), squashFSfile)))
		g.Add(opDownloadISO, herd.WithCallback(ops.DownloadArtifact(artifact.ISOUrl(), isoFile)))
	}

	g.Add(opCopyCloudConfig,
		herd.WithDeps(opPrepareISO),
		herd.WithCallback(func(ctx context.Context) error {
			_, err := copy(cloudConfigFile, filepath.Join(dst, "config.yaml"))
			return err
		}))

	g.Add(opInjectCC,
		herd.WithDeps(opCopyCloudConfig),
		herd.ConditionalOption(func() bool { return !fromImage }, herd.WithDeps(opDownloadISO)),
		herd.ConditionalOption(func() bool { return fromImage }, herd.WithDeps(opGenISO)),
		herd.WithCallback(func(ctx context.Context) error {
			os.Chdir(dst)
			injectedIso := isoFile + ".custom.iso"
			os.Remove(injectedIso)
			out, err := utils.SH(fmt.Sprintf("xorriso -indev %s -outdev %s -map %s /config.yaml -boot_image any replay", isoFile, injectedIso, filepath.Join(dst, "config.yaml")))
			log.Print(out)
			return err
		}))

	if !c.DisableISOboot {
		g.Add(
			opStartHTTPServer,
			herd.Background,
			herd.ConditionalOption(func() bool { return fromImage }, herd.WithDeps(opGenISO, opCopyCloudConfig, opInjectCC)),
			herd.ConditionalOption(func() bool { return !fromImage }, herd.WithDeps(opDownloadISO, opCopyCloudConfig, opInjectCC)),
			herd.WithCallback(ops.ServeArtifacts(":8080", dst)),
		)
	}

	if !c.DisableNetboot {
		g.Add(
			opStartNetboot,
			herd.ConditionalOption(func() bool { return !fromImage }, herd.WithDeps(opDownloadInitrd, opDownloadKernel, opDownloadSquashFS)),
			herd.ConditionalOption(func() bool { return fromImage }, herd.WithDeps(opExtractNetboot)),
			herd.Background,
			herd.WithCallback(func(ctx context.Context) error {
				log.Info().Msgf("Start pixiecore")

				configFile := cloudConfigFile

				cmdLine := `rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID "%s" }} netboot nodepair.enable config_url={{ ID "%s" }} console=tty1 console=ttyS0 console=tty0`
				return netboot.Server(kernelFile, "AuroraBoot", fmt.Sprintf(cmdLine, squashFSfile, configFile), []string{initrdFile}, true)
			},
			),
		)
	}
}

func copy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
