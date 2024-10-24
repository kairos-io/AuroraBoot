package deployer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kairos-io/AuroraBoot/pkg/ops"

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

	opGenRawDisk    = "gen-raw-disk"
	opGenMBRRawDisk = "gen-raw-mbr-disk"

	opExtractSquashFS = "extract-squashfs"

	opConvertGCE       = "convert-gce"
	opConvertVHD       = "convert-vhd"
	opGenARMImages     = "build-arm-image"
	opPrepareARMImages = "prepare_arm"
)

const (
	kairosDefaultArtifactName = "kairos"
)

// RegisterAll register the op dag based on the configuration and the artifact wanted.
// This registers all steps for the top level Auroraboot command.
func RegisterAll(d *Deployer) {
	dst := d.Config.StateDir("build")
	dstNetboot := d.Config.StateDir("netboot")

	listenAddr := ":8080"
	if d.Config.ListenAddr != "" {
		listenAddr = d.Config.ListenAddr
	}

	netbootPort := "8090"
	if d.Config.NetBootHTTPPort != "" {
		netbootPort = d.Config.NetBootHTTPPort
	}
	address := "0.0.0.0"
	if d.Config.NetBootListenAddr != "" {
		netbootPort = d.Config.NetBootListenAddr
	}

	// squashfs, kernel, and initrd names are tied to the output of /netboot.sh (op.ExtractNetboot)
	squashFSfile := filepath.Join(dstNetboot, "kairos.squashfs")
	kernelFile := filepath.Join(dstNetboot, "kairos-kernel")
	initrdFile := filepath.Join(dstNetboot, "kairos-initrd")
	isoFile := filepath.Join(dst, "kairos.iso")
	tmpRootfs := d.Config.StateDir("temp-rootfs")
	fromImage := d.Artifact.ContainerImage != ""
	fromImageOption := func() bool { return fromImage }
	isoOption := func() bool { return !fromImage }
	netbootOption := func() bool { return !d.Config.DisableNetboot }
	netbootReleaseOption := func() bool { return !d.Config.DisableNetboot && !fromImage }
	rawDiskIsSet := d.Config.Disk.VHD || d.Config.Disk.RAW || d.Config.Disk.GCE

	// Pull locak docker daemon if container image starts with docker://
	containerImage := d.Artifact.ContainerImage
	if strings.HasPrefix(containerImage, "docker://") {
		containerImage = strings.ReplaceAll(containerImage, "docker://", "")
	}

	// Preparation steps
	// TODO: Handle errors?
	d.StepPrepTmpRootDir()
	d.StepPrepNetbootDir()
	d.StepPrepISODir()
	d.StepCopyCloudConfig()

	// Ops to generate from container image
	d.Add(opContainerPull,
		herd.EnableIf(fromImageOption),
		herd.WithDeps(opPreparetmproot), herd.WithCallback(ops.PullContainerImage(containerImage, tmpRootfs)))
	d.Add(opGenISO,
		herd.EnableIf(func() bool { return fromImage && !rawDiskIsSet && d.Config.Disk.ARM == nil }),
		herd.WithDeps(opContainerPull, opCopyCloudConfig), herd.WithCallback(ops.GenISO(kairosDefaultArtifactName, tmpRootfs, dst, d.Config.ISO)))
	d.Add(opExtractNetboot,
		herd.EnableIf(func() bool { return fromImage && !d.Config.DisableNetboot }),
		herd.WithDeps(opGenISO), herd.WithCallback(ops.ExtractNetboot(isoFile, dstNetboot, kairosDefaultArtifactName)))

	//TODO: add Validate step
	// Ops to download from releases
	d.Add(opDownloadInitrd,
		herd.EnableIf(netbootReleaseOption),
		herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(d.Artifact.InitrdURL(), initrdFile)))
	d.Add(opDownloadKernel,
		herd.EnableIf(netbootReleaseOption),
		herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(d.Artifact.KernelURL(), kernelFile)))
	d.Add(opDownloadSquashFS,
		herd.EnableIf(func() bool {
			return !d.Config.DisableNetboot && !fromImage || rawDiskIsSet && !fromImage || !fromImage && d.Config.Disk.ARM != nil
		}),
		herd.WithDeps(opPrepareNetboot), herd.WithCallback(ops.DownloadArtifact(d.Artifact.SquashFSURL(), squashFSfile)))
	d.Add(opDownloadISO,
		herd.EnableIf(isoOption),
		herd.WithCallback(ops.DownloadArtifact(d.Artifact.ISOUrl(), isoFile)))

	// Ops to generate disk images

	// Extract SquashFS from released asset to build the raw disk image if needed
	d.Add(opExtractSquashFS,
		herd.EnableIf(func() bool { return rawDiskIsSet && !fromImage }),
		herd.WithDeps(opDownloadSquashFS), herd.WithCallback(ops.ExtractSquashFS(squashFSfile, tmpRootfs)))

	imageOrSquashFS := herd.IfElse(fromImage, herd.WithDeps(opContainerPull), herd.WithDeps(opExtractSquashFS))

	d.Add(opGenRawDisk,
		herd.EnableIf(func() bool { return rawDiskIsSet && d.Config.Disk.ARM == nil && !d.Config.Disk.MBR }),
		imageOrSquashFS,
		herd.WithCallback(ops.GenEFIRawDisk(tmpRootfs, filepath.Join(dst, "disk.raw"))))

	d.Add(opGenMBRRawDisk,
		herd.EnableIf(func() bool { return d.Config.Disk.ARM == nil && d.Config.Disk.MBR }),
		herd.IfElse(isoOption(),
			herd.WithDeps(opDownloadISO), herd.WithDeps(opGenISO),
		),
		herd.IfElse(isoOption(),
			herd.WithCallback(
				ops.GenBIOSRawDisk(d.Config, isoFile, filepath.Join(dst, "disk.raw"))),
			herd.WithCallback(
				ops.GenBIOSRawDisk(d.Config, isoFile, filepath.Join(dst, "disk.raw"))),
		),
	)

	d.Add(opConvertGCE,
		herd.EnableIf(func() bool { return d.Config.Disk.GCE }),
		herd.WithDeps(opGenRawDisk),
		herd.WithCallback(ops.ConvertRawDiskToGCE(filepath.Join(dst, "disk.raw"), filepath.Join(dst, "disk.raw.gce"))))

	d.Add(opConvertVHD,
		herd.EnableIf(func() bool { return d.Config.Disk.VHD }),
		herd.WithDeps(opGenRawDisk),
		herd.WithCallback(ops.ConvertRawDiskToVHD(filepath.Join(dst, "disk.raw"), filepath.Join(dst, "disk.raw.vhd"))))

	// ARM

	d.Add(opGenARMImages,
		herd.EnableIf(func() bool { return d.Config.Disk.ARM != nil && !d.Config.Disk.ARM.PrepareOnly }),
		imageOrSquashFS,
		herd.WithCallback(ops.GenArmDisk(tmpRootfs, filepath.Join(dst, "disk.img"), d.Config)))

	d.Add(opPrepareARMImages,
		herd.EnableIf(func() bool { return d.Config.Disk.ARM != nil && d.Config.Disk.ARM.PrepareOnly }),
		imageOrSquashFS,
		herd.WithCallback(ops.PrepareArmPartitions(tmpRootfs, dst, d.Config)))

	// Inject the data into the ISO
	d.Add(opInjectCC,
		herd.EnableIf(isoOption),
		herd.WithDeps(opCopyCloudConfig),
		herd.ConditionalOption(isoOption, herd.WithDeps(opDownloadISO)),
		herd.WithCallback(ops.InjectISO(dst, isoFile, d.Config.ISO)))

	// Start servers
	d.Add(
		opStartHTTPServer,
		herd.Background,
		herd.EnableIf(func() bool { return !d.Config.DisableISOboot && !d.Config.DisableHTTPServer }),
		herd.IfElse(
			fromImage,
			herd.WithDeps(opGenISO, opCopyCloudConfig),
			herd.WithDeps(opDownloadISO, opCopyCloudConfig, opInjectCC),
		),
		herd.WithCallback(ops.ServeArtifacts(listenAddr, dst)),
	)

	d.Add(
		opStartNetboot,
		herd.EnableIf(netbootOption),
		herd.ConditionalOption(isoOption, herd.WithDeps(opDownloadInitrd, opDownloadKernel, opDownloadSquashFS)),
		herd.ConditionalOption(fromImageOption, herd.WithDeps(opExtractNetboot)),
		herd.Background,
		herd.WithDeps(opCopyCloudConfig),
		herd.WithCallback(
			ops.StartPixiecore(filepath.Join(dst, "config.yaml"), squashFSfile, address, netbootPort, initrdFile, kernelFile, d.Config.NetBoot),
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
