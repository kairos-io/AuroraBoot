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
	"github.com/kairos-io/AuroraBoot/pkg/schema"

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
func Register(g *herd.Graph, artifact schema.ReleaseArtifact, c schema.Config, cloudConfigFile string) {
	dst := c.StateDir("iso")
	dstNetboot := c.StateDir("netboot")

	listenAddr := ":8080"
	if c.ListenAddr != "" {
		listenAddr = c.ListenAddr
	}

	netbootPort := "8090"
	if c.NetBootHTTPPort != "" {
		netbootPort = c.NetBootHTTPPort
	}
	address := "0.0.0.0"
	if c.NetBootListenAddr != "" {
		netbootPort = c.NetBootListenAddr
	}

	// squashfs, kernel, and initrd names are tied to the output of /netboot.sh (op.ExtractNetboot)
	squashFSfile := filepath.Join(dstNetboot, "kairos.squashfs")
	kernelFile := filepath.Join(dstNetboot, "kairos-kernel")
	initrdFile := filepath.Join(dstNetboot, "kairos-initrd")
	isoFile := filepath.Join(dst, "kairos.iso")
	tmpRootfs := c.StateDir("temp-rootfs")
	fromImage := artifact.ContainerImage != ""
	fromImageOption := func() bool { return fromImage }
	isoOption := func() bool { return !fromImage }

	// Pull locak docker daemon if container image starts with docker://
	containerImage := artifact.ContainerImage
	local := false

	if strings.HasPrefix(containerImage, "docker://") {
		local = true
		containerImage = strings.ReplaceAll(containerImage, "docker://", "")
	}

	// Preparation steps
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

	g.Add(opCopyCloudConfig,
		herd.WithDeps(opPrepareISO),
		herd.WithCallback(func(ctx context.Context) error {
			_, err := copy(cloudConfigFile, filepath.Join(dst, "config.yaml"))
			return err
		}))

	// Ops to generate from container image
	g.Add(opContainerPull,
		herd.EnableIf(fromImageOption),
		herd.WithDeps(opPreparetmproot), herd.WithCallback(ops.PullContainerImage(containerImage, tmpRootfs, local)))
	g.Add(opGenISO,
		herd.EnableIf(fromImageOption),
		herd.WithDeps(opContainerPull, opCopyCloudConfig), herd.WithCallback(ops.GenISO(kairosDefaultArtifactName, tmpRootfs, dst, c.ISO)))
	g.Add(opExtractNetboot,
		herd.EnableIf(func() bool { return fromImage && !c.DisableNetboot }),
		herd.WithDeps(opGenISO), herd.WithCallback(ops.ExtractNetboot(isoFile, dstNetboot, kairosDefaultArtifactName)))

	//TODO: add Validate step
	// Ops to download from releases
	g.Add(opDownloadInitrd,
		herd.EnableIf(isoOption),
		herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(artifact.InitrdURL(), initrdFile)))
	g.Add(opDownloadKernel,
		herd.EnableIf(isoOption),
		herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(artifact.KernelURL(), kernelFile)))
	g.Add(opDownloadSquashFS,
		herd.EnableIf(isoOption),
		herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(artifact.SquashFSURL(), squashFSfile)))
	g.Add(opDownloadISO,
		herd.EnableIf(isoOption),
		herd.WithCallback(ops.DownloadArtifact(artifact.ISOUrl(), isoFile)))

	// Inject the data into the ISO
	g.Add(opInjectCC,
		herd.EnableIf(isoOption),
		herd.WithDeps(opCopyCloudConfig),
		herd.ConditionalOption(isoOption, herd.WithDeps(opDownloadISO)),
		herd.WithCallback(ops.InjectISO(dst, isoFile, c.ISO)))

	// Start servers
	g.Add(
		opStartHTTPServer,
		herd.Background,
		herd.EnableIf(func() bool { return !c.DisableISOboot && !c.DisableHTTPServer }),
		herd.IfElse(
			fromImage,
			herd.WithDeps(opGenISO, opCopyCloudConfig),
			herd.WithDeps(opDownloadISO, opCopyCloudConfig, opInjectCC),
		),
		herd.WithCallback(ops.ServeArtifacts(listenAddr, dst)),
	)

	g.Add(
		opStartNetboot,
		herd.EnableIf(func() bool { return !c.DisableNetboot }),
		herd.ConditionalOption(isoOption, herd.WithDeps(opDownloadInitrd, opDownloadKernel, opDownloadSquashFS)),
		herd.ConditionalOption(fromImageOption, herd.WithDeps(opExtractNetboot)),
		herd.Background,
		herd.WithCallback(func(ctx context.Context) error {
			log.Info().Msgf("Start pixiecore")

			configFile := cloudConfigFile

			cmdLine := `rd.neednet=1 ip=dhcp rd.cos.disable root=live:{{ ID "%s" }} netboot nodepair.enable config_url={{ ID "%s" }} console=tty1 console=ttyS0 console=tty0`
			return netboot.Server(kernelFile, "AuroraBoot", fmt.Sprintf(cmdLine, squashFSfile, configFile), address, netbootPort, []string{initrdFile}, true)
		},
		),
	)
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
