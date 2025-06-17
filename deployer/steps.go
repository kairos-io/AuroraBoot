package deployer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hashicorp/go-multierror"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/constants"

	"github.com/kairos-io/AuroraBoot/pkg/ops"
	"github.com/spectrocloud-labs/herd"
)

func (d *Deployer) StepPrepNetbootDir() error {
	return d.Add(constants.OpPrepareNetboot,
		herd.WithDeps(constants.OpPrepareDestination),
		herd.WithCallback(
			func(ctx context.Context) error {
				internal.Log.Logger.Info().Str("destination", d.dstNetboot()).Msg("Preparing temp netboot directory")
				err := os.RemoveAll(d.dstNetboot())
				if err != nil {
					internal.Log.Logger.Error().Err(err).Msg("Failed to remove temp netboot dir")
					return err
				}
				return os.MkdirAll(d.dstNetboot(), 0755)
			},
		))
}

func (d *Deployer) StepPrepTmpRootDir() error {
	return d.Add(constants.OpPreparetmproot,
		herd.WithDeps(constants.OpPrepareDestination),
		herd.WithCallback(
			func(ctx context.Context) error {
				internal.Log.Logger.Info().Str("destination", d.tmpRootFs()).Msg("Preparing temp rootfs directory")
				err := os.RemoveAll(d.tmpRootFs())
				if err != nil {
					internal.Log.Logger.Error().Err(err).Msg("Failed to remove temp rootfs")
					return err
				}
				return os.MkdirAll(d.tmpRootFs(), 0755)
			},
		))
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

// StepPrepDestination prepares the destination directory for the rest of the steps.
func (d *Deployer) StepPrepDestination() error {
	return d.Add(constants.OpPrepareDestination, herd.WithCallback(func(ctx context.Context) error {
		internal.Log.Logger.Info().Str("destination", d.destination()).Msg("Preparing destination temporal directory")
		if d.destination() == "" {
			internal.Log.Logger.Error().Msg("Destination directory is not set, cannot prepare ISO directory")
			return fmt.Errorf("destination directory is not set")
		}
		return os.MkdirAll(d.destination(), 0755)
	}))
}

func (d *Deployer) StepCopyCloudConfig() error {
	return d.Add(constants.OpCopyCloudConfig,
		herd.WithDeps(constants.OpPrepareDestination),
		herd.WithCallback(func(ctx context.Context) error {
			internal.Log.Logger.Info().Str("cloudConfig", d.Config.CloudConfig).Msg("Copying cloud config")
			if _, err := os.Stat(d.destination()); err != nil && os.IsNotExist(err) {
				internal.Log.Logger.Error().Err(err).Msg("Destination directory does not exist, creating it")
				if err := os.MkdirAll(d.destination(), 0755); err != nil {
					return err
				}
			}

			return os.WriteFile(d.cloudConfigPath(), []byte(d.Config.CloudConfig), 0600)
		}))
}

func (d *Deployer) StepDumpSource() error {
	// Ops to generate from container image
	return d.Add(constants.OpDumpSource,
		herd.EnableIf(d.fromImage),
		herd.WithDeps(constants.OpPreparetmproot), herd.WithCallback(ops.DumpSource(d.Artifact.ContainerImage, d.tmpRootFs())))
}

func (d *Deployer) StepGenISO() error {
	return d.Add(constants.OpGenISO,
		herd.EnableIf(func() bool { return d.fromImage() && !d.rawDiskIsSet() }),
		herd.WithDeps(constants.OpDumpSource, constants.OpCopyCloudConfig, constants.OpPrepareDestination), herd.WithCallback(ops.GenISO(d.tmpRootFs(), d.destination(), d.Config.ISO)))
}

func (d *Deployer) StepDownloadISO() error {
	return d.Add(constants.OpDownloadISO,
		herd.EnableIf(func() bool { return !d.rawDiskIsSet() && d.isoOption() }),
		herd.WithDeps(constants.OpPrepareDestination),
		herd.WithCallback(ops.DownloadArtifact(d.Artifact.ISOUrl(), d.getIsoFile))) // This is okay to call the getIsoFile function here as we want a destination for the ISO file
}

// StepExtractNetboot Extract netboot artifacts from the ISO file
// Should trigger if netboot is enabled and the ISO is generated or downloaded.
// Add conditional dependencies to ensure the ISO is available before extracting netboot files.
// 2 different conditions, one for the ISO download and one for the ISO generation. The check its the same as the one in StepGenISO/StepDownloadISO
func (d *Deployer) StepExtractNetboot() error {
	return d.Add(constants.OpExtractNetboot,
		herd.EnableIf(func() bool { return !d.Config.DisableNetboot && !d.rawDiskIsSet() }),
		herd.ConditionalOption(func() bool { return d.isoOption() }, herd.WithDeps(constants.OpDownloadISO)),
		herd.ConditionalOption(func() bool { return d.fromImage() }, herd.WithDeps(constants.OpGenISO)),
		herd.WithDeps(constants.OpGenISO), herd.WithCallback(ops.ExtractNetboot(d.getIsoFile, d.dstNetboot, d.Config.ISO.Name)))
}

// StepExtractSquashFS Extract SquashFS from released asset to build the raw disk image if needed
func (d *Deployer) StepExtractSquashFS() error {
	return d.Add(constants.OpExtractSquashFS,
		herd.EnableIf(func() bool { return d.rawDiskIsSet() && !d.fromImage() }),
		herd.WithDeps(), herd.WithCallback(ops.ExtractSquashFS(d.squashFSfile(), d.tmpRootFs())))
}

// StepGenRawDisk Generate the raw disk image.
// Enabled if is explicitly set the disk.efi or disk.vhd or disk.gce as they depend on the efi disk
func (d *Deployer) StepGenRawDisk() error {
	return d.Add(constants.OpGenEFIRawDisk,
		herd.EnableIf(func() bool { return d.Config.Disk.EFI || d.Config.Disk.GCE || d.Config.Disk.VHD }),
		d.imageOrSquashFS(),
		herd.WithCallback(ops.GenEFIRawDisk(d.tmpRootFs(), d.rawDiskPath(), d.rawDiskSize(), d.rawDiskStateSize())))
}

func (d *Deployer) StepGenMBRRawDisk() error {
	return d.Add(constants.OpGenBIOSRawDisk,
		herd.EnableIf(func() bool { return d.Config.Disk.BIOS }),
		d.imageOrSquashFS(),
		herd.WithCallback(ops.GenBiosRawDisk(d.tmpRootFs(), d.rawDiskPath(), d.rawDiskSize(), d.rawDiskStateSize())))
}

func (d *Deployer) StepConvertGCE() error {
	return d.Add(constants.OpConvertGCE,
		herd.EnableIf(func() bool { return d.Config.Disk.GCE }),
		herd.WithDeps(constants.OpGenEFIRawDisk),
		herd.WithCallback(ops.ConvertRawDiskToGCE(d.rawDiskPath())))
}

func (d *Deployer) StepConvertVHD() error {
	return d.Add(constants.OpConvertVHD,
		herd.EnableIf(func() bool { return d.Config.Disk.VHD }),
		herd.WithDeps(constants.OpGenEFIRawDisk),
		herd.WithCallback(ops.ConvertRawDiskToVHD(d.rawDiskPath())))
}

func (d *Deployer) StepInjectCC() error {
	return d.Add(constants.OpInjectCC,
		herd.EnableIf(func() bool { return !d.rawDiskIsSet() && d.isoOption() }),
		herd.WithDeps(constants.OpCopyCloudConfig),
		herd.ConditionalOption(d.isoOption, herd.WithDeps(constants.OpDownloadISO)),
		herd.ConditionalOption(d.fromImage, herd.WithDeps(constants.OpGenISO)),
		herd.WithCallback(ops.InjectISO(d.destination(), d.getIsoFile, d.Config.ISO)))
}

func (d *Deployer) StepStartHTTPServer() error {
	return d.Add(constants.OpStartHTTPServer,
		herd.Background,
		herd.EnableIf(func() bool { return !d.Config.DisableISOboot && !d.Config.DisableHTTPServer }),
		herd.IfElse(
			d.fromImage(),
			herd.WithDeps(constants.OpGenISO, constants.OpCopyCloudConfig, constants.OpInjectCC),
			herd.WithDeps(constants.OpDownloadISO, constants.OpCopyCloudConfig, constants.OpInjectCC),
		),
		herd.WithCallback(ops.ServeArtifacts(d.listenAddr(), d.destination())),
	)
}

func (d *Deployer) StepStartNetboot() error {
	return d.Add(constants.OpStartNetboot,
		herd.EnableIf(d.netbootOption),
		herd.Background,
		herd.WithDeps(constants.OpExtractNetboot, constants.OpCopyCloudConfig),
		herd.WithCallback(
			ops.StartPixiecore(d.cloudConfigPath(), d.netBootListenAddr(), d.netbootPort(), d.squashFSfile, d.initrdFile, d.kernelFile, d.Config.NetBoot),
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

// getIsoFile returns the path to the ISO file.
// It first checks for the default name in the destination directory,
// and if not found, it searches for any ISO file in the destination directory.
func (d *Deployer) getIsoFile() string {
	defaultIsoPath := filepath.Join(d.destination(), "kairos.iso")
	// This is to look for the ISO file in the destination directory
	if _, err := os.Stat(defaultIsoPath); err == nil {
		internal.Log.Logger.Info().Str("isoFile", defaultIsoPath).Msg("Found existing ISO file")
		return defaultIsoPath
	}
	// If its not the default name, we search for the ISO file in the destination directory
	files, err := os.ReadDir(d.destination())
	if err != nil {
		internal.Log.Logger.Debug().Msg("Failed to read destination directory, falling back to default ISO name")
		return defaultIsoPath // fallback to the default name
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if filepath.Ext(file.Name()) == ".iso" {
			internal.Log.Logger.Info().Str("isoFile", file.Name()).Msg("Found existing ISO file")
			return filepath.Join(d.destination(), file.Name())
		}
	}
	internal.Log.Logger.Info().Str("isoFile", defaultIsoPath).Msg("No ISO file found, falling back to default ISO name")
	return defaultIsoPath
}

func (d *Deployer) dstNetboot() string {
	return d.Config.StateDir("netboot")
}

// Returns true if any of the options for raw disk is set
func (d *Deployer) rawDiskIsSet() bool {
	return d.Config.Disk.VHD || d.Config.Disk.EFI || d.Config.Disk.GCE || d.Config.Disk.BIOS
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
	return herd.IfElse(d.fromImage(), herd.WithDeps(constants.OpDumpSource), herd.WithDeps(constants.OpExtractSquashFS))
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

func (d *Deployer) rawDiskStateSize() int64 {
	if d.Config.Disk.StateSize == "" {
		return 0
	}
	sizeInt, err := strconv.ParseInt(d.Config.Disk.StateSize, 10, 64)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("arg", d.Config.Disk.StateSize).Msg("Failed to parse disk state size, setting value to 0")
		return 0
	}
	return sizeInt
}
