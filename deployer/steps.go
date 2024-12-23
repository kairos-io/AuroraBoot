package deployer

import (
	"context"
	"github.com/hashicorp/go-multierror"
	"github.com/kairos-io/AuroraBoot/internal"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/kairos-io/AuroraBoot/pkg/ops"
	"github.com/spectrocloud-labs/herd"
)

type callback func(context.Context) error

// logStepTime logs the start and end time of a step
// and the duration it took to complete
func logStepTime(stepName string, stepFunc callback) callback {
	return func(ctx context.Context) error {
		start := time.Now()
		internal.Log.Logger.Trace().Str("step", stepName).Time("start", start).Msg("Starting step")
		err := stepFunc(ctx)
		end := time.Now()
		duration := end.Sub(start)
		if err != nil {
			internal.Log.Logger.Trace().Err(err).Str("step", stepName).Time("start", start).Time("end", end).Str("duration", duration.String()).Msg("Step failed")
		} else {
			internal.Log.Logger.Trace().Str("step", stepName).Time("start", start).Time("end", end).Str("duration", duration.String()).Msg("Completed step")
		}
		return err
	}
}

func (d *Deployer) StepPrepNetbootDir() error {
	return d.Add(opPrepareNetboot, herd.WithCallback(
		logStepTime(opPrepareNetboot, func(ctx context.Context) error {
			err := os.RemoveAll(d.dstNetboot())
			if err != nil {
				internal.Log.Logger.Error().Err(err).Msg("Failed to remove temp netboot dir")
				return err
			}
			return os.MkdirAll(d.dstNetboot(), 0755)
		}),
	))
}

func (d *Deployer) StepPrepTmpRootDir() error {
	return d.Add(opPreparetmproot, herd.WithCallback(
		logStepTime(opPreparetmproot, func(ctx context.Context) error {
			err := os.RemoveAll(d.tmpRootFs())
			if err != nil {
				internal.Log.Logger.Error().Err(err).Msg("Failed to remove temp rootfs")
				return err
			}
			return os.MkdirAll(d.tmpRootFs(), 0755)
		},
		)))
}

// CleanTmpDirs removes the temp rootfs and netboot directories when finished to not leave things around
func (d *Deployer) CleanTmpDirs() error {
	var err *multierror.Error
	err = multierror.Append(err, os.RemoveAll(d.tmpRootFs()))
	if err.ErrorOrNil() != nil {
		internal.Log.Logger.Error().Err(err).Msg("Failed to remove temp rootfs")
	}

	err = multierror.Append(err, os.RemoveAll(d.dstNetboot()))
	return err.ErrorOrNil()
}

func (d *Deployer) StepPrepISODir() error {
	return d.Add(opPrepareISO, herd.WithCallback(logStepTime(opPrepareISO, func(ctx context.Context) error {
		return os.MkdirAll(d.destination(), 0755)
	})))
}

func (d *Deployer) StepCopyCloudConfig() error {
	return d.Add(opCopyCloudConfig,
		herd.WithDeps(opPrepareISO),
		herd.WithCallback(logStepTime(opCopyCloudConfig, func(ctx context.Context) error {
			return os.WriteFile(d.cloudConfigPath(), []byte(d.Config.CloudConfig), 0600)
		})))
}

func (d *Deployer) StepDumpSource() error {
	// Ops to generate from container image
	return d.Add(opDumpSource,
		herd.EnableIf(d.fromImage),
		herd.WithDeps(opPreparetmproot), herd.WithCallback(logStepTime(opDumpSource, ops.DumpSource(d.Artifact.ContainerImage, d.tmpRootFs()))))
}

func (d *Deployer) StepGenISO() error {
	return d.Add(opGenISO,
		herd.EnableIf(func() bool { return d.fromImage() && !d.rawDiskIsSet() && d.Config.Disk.ARM == nil }),
		herd.WithDeps(opDumpSource, opCopyCloudConfig), herd.WithCallback(logStepTime(opGenISO, ops.GenISO(d.tmpRootFs(), d.destination(), d.Config.ISO))))
}

func (d *Deployer) StepExtractNetboot() error {
	return d.Add(opExtractNetboot,
		herd.EnableIf(func() bool { return d.fromImage() && !d.Config.DisableNetboot }),
		herd.WithDeps(opGenISO), herd.WithCallback(logStepTime(opExtractNetboot, ops.ExtractNetboot(d.isoFile(), d.dstNetboot(), d.Config.ISO.Name))))
}

func (d *Deployer) StepDownloadInitrd() error {
	return d.Add(opDownloadInitrd,
		herd.EnableIf(d.netbootReleaseOption),
		herd.WithDeps(opPrepareNetboot), herd.WithCallback(logStepTime(opDownloadInitrd, ops.DownloadArtifact(d.Artifact.InitrdURL(), d.initrdFile()))))
}

func (d *Deployer) StepDownloadKernel() error {
	return d.Add(opDownloadKernel,
		herd.EnableIf(d.netbootReleaseOption),
		herd.WithDeps(opPrepareNetboot), herd.WithCallback(logStepTime(opDownloadKernel, ops.DownloadArtifact(d.Artifact.KernelURL(), d.kernelFile()))))
}

func (d *Deployer) StepDownloadSquashFS() error {
	return d.Add(opDownloadSquashFS,
		herd.EnableIf(func() bool {
			return !d.Config.DisableNetboot && !d.fromImage() || d.rawDiskIsSet() && !d.fromImage() || !d.fromImage() && d.Config.Disk.ARM != nil
		}),
		herd.WithDeps(opPrepareNetboot), herd.WithCallback(logStepTime(opDownloadSquashFS, ops.DownloadArtifact(d.Artifact.SquashFSURL(), d.squashFSfile()))))
}

func (d *Deployer) StepDownloadISO() error {
	return d.Add(opDownloadISO,
		herd.EnableIf(d.isoOption),
		herd.WithCallback(logStepTime(opDownloadISO, ops.DownloadArtifact(d.Artifact.ISOUrl(), d.isoFile()))))
}

// Extract SquashFS from released asset to build the raw disk image if needed
func (d *Deployer) StepExtractSquashFS() error {
	return d.Add(opExtractSquashFS,
		herd.EnableIf(func() bool { return d.rawDiskIsSet() && !d.fromImage() }),
		herd.WithDeps(opDownloadSquashFS), herd.WithCallback(logStepTime(opExtractSquashFS, ops.ExtractSquashFS(d.squashFSfile(), d.tmpRootFs()))))
}

func (d *Deployer) StepGenRawDisk() error {
	return d.Add(opGenRawDisk,
		herd.EnableIf(func() bool { return d.rawDiskIsSet() && d.Config.Disk.ARM == nil && !d.Config.Disk.MBR }),
		d.imageOrSquashFS(),
		herd.WithCallback(logStepTime(opGenRawDisk, ops.GenEFIRawDisk(d.tmpRootFs(), d.rawDiskPath(), d.rawDiskSize()))))
}

func (d *Deployer) StepGenMBRRawDisk() error {
	return d.Add(opGenMBRRawDisk,
		herd.EnableIf(func() bool { return d.Config.Disk.ARM == nil && d.Config.Disk.MBR }),
		herd.IfElse(d.isoOption(),
			herd.WithDeps(opDownloadISO), herd.WithDeps(opGenISO),
		),
		herd.IfElse(d.isoOption(),
			herd.WithCallback(
				logStepTime(opGenMBRRawDisk, ops.GenBIOSRawDisk(d.Config, d.isoFile(), d.rawDiskPath()))),
			herd.WithCallback(
				logStepTime(opGenMBRRawDisk, ops.GenBIOSRawDisk(d.Config, d.isoFile(), d.rawDiskPath()))),
		),
	)
}

func (d *Deployer) StepConvertGCE() error {
	return d.Add(opConvertGCE,
		herd.EnableIf(func() bool { return d.Config.Disk.GCE }),
		herd.WithDeps(opGenRawDisk),
		herd.WithCallback(logStepTime(opConvertGCE, ops.ConvertRawDiskToGCE(d.rawDiskPath()))))
}

func (d *Deployer) StepConvertVHD() error {
	return d.Add(opConvertVHD,
		herd.EnableIf(func() bool { return d.Config.Disk.VHD }),
		herd.WithDeps(opGenRawDisk),
		herd.WithCallback(logStepTime(opConvertVHD, ops.ConvertRawDiskToVHD(d.rawDiskPath()))))
}

func (d *Deployer) StepGenARMImages() error {
	return d.Add(opGenARMImages,
		herd.EnableIf(func() bool { return d.Config.Disk.ARM != nil && !d.Config.Disk.ARM.PrepareOnly }),
		d.imageOrSquashFS(),
		herd.WithCallback(logStepTime(opGenARMImages, ops.GenArmDisk(d.tmpRootFs(), d.diskImgPath(), d.Config))))
}

func (d *Deployer) StepPrepareARMImages() error {
	return d.Add(opPrepareARMImages,
		herd.EnableIf(func() bool { return d.Config.Disk.ARM != nil && d.Config.Disk.ARM.PrepareOnly }),
		d.imageOrSquashFS(),
		herd.WithCallback(logStepTime(opPrepareARMImages, ops.PrepareArmPartitions(d.tmpRootFs(), d.destination(), d.Config))))
}

func (d *Deployer) StepInjectCC() error {
	return d.Add(opInjectCC,
		herd.EnableIf(d.isoOption),
		herd.WithDeps(opCopyCloudConfig),
		herd.ConditionalOption(d.isoOption, herd.WithDeps(opDownloadISO)),
		herd.WithCallback(logStepTime(opInjectCC, ops.InjectISO(d.destination(), d.isoFile(), d.Config.ISO))))
}

func (d *Deployer) StepStartHTTPServer() error {
	return d.Add(
		opStartHTTPServer,
		herd.Background,
		herd.EnableIf(func() bool { return !d.Config.DisableISOboot && !d.Config.DisableHTTPServer }),
		herd.IfElse(
			d.fromImage(),
			herd.WithDeps(opGenISO, opCopyCloudConfig),
			herd.WithDeps(opDownloadISO, opCopyCloudConfig, opInjectCC),
		),
		herd.WithCallback(ops.ServeArtifacts(d.listenAddr(), d.destination())),
	)
}

func (d *Deployer) StepStartNetboot() error {
	return d.Add(
		opStartNetboot,
		herd.EnableIf(d.netbootOption),
		herd.ConditionalOption(d.isoOption, herd.WithDeps(opDownloadInitrd, opDownloadKernel, opDownloadSquashFS)),
		herd.ConditionalOption(d.fromImage, herd.WithDeps(opExtractNetboot)),
		herd.Background,
		herd.WithDeps(opCopyCloudConfig),
		herd.WithCallback(
			ops.StartPixiecore(d.cloudConfigPath(), d.squashFSfile(), d.netBootListenAddr(), d.netbootPort(), d.initrdFile(), d.kernelFile(), d.Config.NetBoot),
		),
	)
}

func (d *Deployer) fromImage() bool {
	return d.Artifact.ContainerImage != ""
}

func (d *Deployer) tmpRootFs() string {
	return d.Config.StateDir("temp-rootfs")
}

func (d *Deployer) destination() string {
	return d.Config.State
}

func (d *Deployer) isoFile() string {
	return filepath.Join(d.destination(), "kairos.iso")
}

func (d *Deployer) dstNetboot() string {
	return d.Config.StateDir("netboot")
}

func (d *Deployer) rawDiskIsSet() bool {
	return d.Config.Disk.VHD || d.Config.Disk.RAW || d.Config.Disk.GCE
}

func (d *Deployer) netbootReleaseOption() bool {
	return !d.Config.DisableNetboot && !d.fromImage()
}

func (d *Deployer) initrdFile() string {
	return filepath.Join(d.dstNetboot(), "kairos-initrd")
}

func (d *Deployer) kernelFile() string {
	return filepath.Join(d.dstNetboot(), "kairos-kernel")
}

func (d *Deployer) squashFSfile() string {
	return filepath.Join(d.dstNetboot(), "kairos.squashfs")
}

func (d *Deployer) isoOption() bool {
	return !d.fromImage()
}

func (d *Deployer) imageOrSquashFS() herd.OpOption {
	return herd.IfElse(d.fromImage(), herd.WithDeps(opDumpSource), herd.WithDeps(opExtractSquashFS))
}

func (d *Deployer) cloudConfigPath() string {
	return filepath.Join(d.destination(), "config.yaml")
}

// Return only the path to the output dir, the image name is generated based on the rootfs
func (d *Deployer) rawDiskPath() string {
	return d.destination()
}

func (d *Deployer) diskImgPath() string {
	return filepath.Join(d.destination(), "disk.img")
}

func (d *Deployer) listenAddr() string {
	listenAddr := ":8080"
	if d.Config.ListenAddr != "" {
		listenAddr = d.Config.ListenAddr
	}

	return listenAddr
}

func (d *Deployer) netbootPort() string {
	netbootPort := "8090"
	if d.Config.NetBootHTTPPort != "" {
		netbootPort = d.Config.NetBootHTTPPort
	}

	return netbootPort
}

func (d *Deployer) netBootListenAddr() string {
	address := "0.0.0.0"
	if d.Config.NetBootListenAddr != "" {
		address = d.Config.NetBootListenAddr
	}

	return address
}

func (d *Deployer) netbootOption() bool {
	// squashfs, kernel, and initrd names are tied to the output of /netboot.sh (op.ExtractNetboot)
	return !d.Config.DisableNetboot
}

func (d *Deployer) rawDiskSize() uint64 {
	// parse the string into a uint64
	// the size is in Mb
	if d.Config.Disk.Size == "" {
		return 0
	}
	sizeInt, err := strconv.ParseUint(d.Config.Disk.Size, 10, 64)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("arg", d.Config.Disk.Size).Msg("Failed to parse disk size, setting value to 0")
		return 0
	}
	return sizeInt
}
