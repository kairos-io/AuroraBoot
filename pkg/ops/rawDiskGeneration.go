package ops

import (
	"fmt"
	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/partition"
	"github.com/diskfs/go-diskfs/partition/gpt"
	"github.com/gofrs/uuid"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/constants"
	"github.com/kairos-io/AuroraBoot/pkg/utils"
	"github.com/kairos-io/kairos-agent/v2/pkg/config"
	agentConstants "github.com/kairos-io/kairos-agent/v2/pkg/constants"
	"github.com/kairos-io/kairos-agent/v2/pkg/elemental"
	v1 "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
	agentUtils "github.com/kairos-io/kairos-agent/v2/pkg/utils"
	fsutils "github.com/kairos-io/kairos-agent/v2/pkg/utils/fs"
	sdkUtils "github.com/kairos-io/kairos-sdk/utils"
	"github.com/mudler/yip/pkg/schema"
	"github.com/twpayne/go-vfs/v5"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// What it does
// Create images for all three partitions
// EFI, OEM, Recovery
// EFI is a fat32 partition with the grub artifacts
// OEM is an ext2 partition with the cloud config
// Recovery is an ext2 partition with the rootfs in an image file called recovery.img under cOS/
// Mount them with loop devices and copy the artifacts to them
// Once the images are created, they are concatenated together with a GPT partition table
// The final image is then truncated to the nearest sector size
// TODO: Add BIOS support
// TODO: Add testing
// The methods are public becuase they may be reused in the future for nvidia images which create the partitions separately
// TODO: Check if they need to be public, otherwise they can be private

type RawImage struct {
	CloudConfig string               // cloud config to copy to the oem partition, if none provided a default one will be created with the kairos user
	Source      string               // Source image to copy the artifacts from, which will be the rootfs in the final image
	Output      string               // Output image destination dir. Final image name will be based on the contents of the source /etc/kairos-release file
	FinalSize   uint64               // Final size of the disk image in MB
	rootfsDir   string               // Rootfs dir which contains the (maybe) extracted rootfs
	tmpDir      string               // A temp dir to do all work on
	elemental   *elemental.Elemental // Elemental instance to use for the operations
	efi         bool                 // If the image should be EFI or BIOS
	config      *config.Config       // config to use for the operations
}

// NewEFIRawImage creates a new RawImage struct
// config is initialized with a default config to use the standard logger
func NewEFIRawImage(source, output string, finalsize uint64) *RawImage {
	cfg := config.NewConfig(config.WithLogger(internal.Log))
	return &RawImage{efi: true, config: cfg, Source: source, Output: output, elemental: elemental.NewElemental(cfg), FinalSize: finalsize}
}

func NewBiosRawImage(source, output string, finalsize uint64) *RawImage {
	cfg := config.NewConfig(config.WithLogger(internal.Log))
	return &RawImage{efi: false, config: cfg, Source: source, Output: output, elemental: elemental.NewElemental(cfg), FinalSize: finalsize}
}

// CreateOemPartitionImage creates an OEM partition image with the given cloud config
func (r *RawImage) CreateOemPartitionImage(recoveryImagePath string) (string, error) {
	// Create a temp dir for copying the files to
	tmpDirOem := filepath.Join(r.tmpDir, "oem")
	err := fsutils.MkdirAll(r.config.Fs, tmpDirOem, 0755)
	defer r.config.Fs.RemoveAll(tmpDirOem)

	// This is where the oem partition will be mounted to copy the files to
	tmpDirOemMount := filepath.Join(r.tmpDir, "oem-mount")
	err = fsutils.MkdirAll(r.config.Fs, tmpDirOemMount, 0755)
	defer r.config.Fs.RemoveAll(tmpDirOemMount)

	// Copy the cloud config to the oem partition if htere is any
	if r.CloudConfig != "" {
		err = fsutils.Copy(r.config.Fs, r.CloudConfig, filepath.Join(tmpDirOem, "90_custom.yaml"))
	} else {
		// Create a default cloud config yaml with at least a user
		err = r.config.Fs.WriteFile(filepath.Join(tmpDirOem, "90_custom.yaml"), []byte(constants.DefaultCloudConfig), 0o644)
	}

	// Set the grubenv to boot into recovery
	err = agentUtils.SetPersistentVariables(filepath.Join(tmpDirOem, "grubenv"), map[string]string{"next_entry": "recovery"}, r.config.Fs)
	if err != nil {
		return "", err
	}

	resetCloudInit := "01_reset.yaml"

	// Calculate the size of the state partition based on the recovery image size
	info, err := r.config.Fs.Stat(recoveryImagePath)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("source", recoveryImagePath).Msg("failed to stat recovery image")
		return "", err
	}

	size := (info.Size()*3 + 100*1024*1024) / (1024 * 1024)
	internal.Log.Logger.Debug().Int64("size", size).Msg("calculated state partition size")

	// Create a reset config
	// This:
	// - Adds a state partition with the calculated size
	// - Adds a persistent partition with the rest of the disk
	// - If the recovery mode file is present, it will run the reset command unattended
	// - If the reset cloud init file is present, it will remove it. Magic! So we dont get any traces of the extra config for raw images
	conf := &schema.YipConfig{
		Name: "Expand disk layout",
		Stages: map[string][]schema.Stage{
			"rootfs.before": {
				schema.Stage{
					Name: "Add state partition",
					Layout: schema.Layout{
						Device: &schema.Device{
							Label: agentConstants.RecoveryLabel,
						},
						Parts: []schema.Partition{
							{
								FSLabel:    agentConstants.StateLabel,
								Size:       uint(size),
								PLabel:     agentConstants.StatePartName,
								FileSystem: agentConstants.LinuxImgFs,
							},
						},
					},
				}, schema.Stage{
					Name: "Add persistent partition",
					Layout: schema.Layout{
						Device: &schema.Device{
							Label: agentConstants.RecoveryLabel,
						},
						Parts: []schema.Partition{
							{
								FSLabel:    agentConstants.PersistentLabel,
								Size:       0, // It will get expanded to the end of the disk
								PLabel:     agentConstants.PersistentPartName,
								FileSystem: agentConstants.LinuxImgFs,
							},
						},
					},
				},
			}, "network": {
				schema.Stage{
					If:   `[ -f "/run/cos/recovery_mode" ]`,
					Name: "Run auto reset",
					Commands: []string{
						"kairos-agent --debug reset --unattended --reboot",
					},
				},
			}, "after-reset": {
				schema.Stage{
					If:   `[ -f "/oem/` + resetCloudInit + `" ]`,
					Name: "Auto remove this file",
					Commands: []string{
						fmt.Sprintf("rm /oem/%s", resetCloudInit),
					},
				},
			},
		},
	}
	yipYAML, err := yaml.Marshal(conf)
	// Save the cloud config
	err = r.config.Fs.WriteFile(filepath.Join(tmpDirOem, resetCloudInit), yipYAML, 0o644)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", filepath.Join(tmpDirOem, resetCloudInit)).Msg("failed to write cloud config")
		return "", err
	}

	OemPartitionImage := v1.Image{
		File:       filepath.Join(r.tmpDir, "oem.img"),
		FS:         agentConstants.LinuxImgFs,
		Label:      agentConstants.OEMLabel,
		Size:       64,
		Source:     v1.NewDirSrc(tmpDirOem),
		MountPoint: tmpDirOemMount,
	}

	// Deploy the source to the image
	_, err = r.elemental.DeployImageNodirs(&OemPartitionImage, false)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("source", r.Source).Interface("image", OemPartitionImage).Msg("failed to create oem image")
		return "", err
	}

	// return the created image file
	return OemPartitionImage.File, nil
}

// CreateRecoveryPartitionImage creates a recovery partition image with the given source
// The source expects to be a directory with the rootfs to generate a squashfs from
// This generates a recovery.img with the rootfs on in under /cOS/
// It also contains the final grub.cfg and grubenv_first
func (r *RawImage) CreateRecoveryPartitionImage() (string, error) {
	// Create a temp dir for mounting the image to
	tmpDirRecoveryImage := filepath.Join(r.tmpDir, "recovery-img")
	err := fsutils.MkdirAll(r.config.Fs, tmpDirRecoveryImage, 0755)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", tmpDirRecoveryImage).Msg("failed to create temp dir")
		return "", err
	}
	defer r.config.Fs.RemoveAll(tmpDirRecoveryImage)

	// Create a dir to store the recovery partition contents
	tmpDirRecovery := filepath.Join(r.tmpDir, "recovery")
	err = fsutils.MkdirAll(r.config.Fs, tmpDirRecovery, 0755)
	defer r.config.Fs.RemoveAll(tmpDirRecovery)

	err = fsutils.MkdirAll(r.config.Fs, filepath.Join(tmpDirRecovery, "cOS"), 0755)

	recoveryImage := v1.Image{
		File:       filepath.Join(tmpDirRecovery, "cOS", agentConstants.RecoveryImgFile),
		FS:         agentConstants.LinuxImgFs,
		Label:      agentConstants.SystemLabel,
		Source:     v1.NewDirSrc(r.rootfsDir),
		MountPoint: tmpDirRecoveryImage,
	}
	size, _ := config.GetSourceSize(r.config, recoveryImage.Source)
	recoveryImage.Size = uint(size)

	_, err = r.elemental.DeployImage(&recoveryImage, false)
	// Create recovery.squash from the rootfs into the recovery partition under cOS/
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("source", r.Source).Interface("image", recoveryImage).Msg("failed to create recovery image")
		return "", err
	}

	// TODO: Copy the grub artifacts to the recovery partition under grub2/ so they can be used by the EFI/BIOS grub
	// contents come form https://github.com/kairos-io/packages/blob/main/packages/static/grub-config/files/grub.cfg

	// Copy the grub.cfg from the rootfs into the recovery partition
	internal.Log.Logger.Debug().Str("source", r.rootfsDir).Str("target", filepath.Join(tmpDirRecovery, filepath.Dir(agentConstants.GrubConf))).Msg("Copying grub.cfg")
	err = fsutils.MkdirAll(r.config.Fs, filepath.Join(tmpDirRecovery, filepath.Dir(agentConstants.GrubConf)), 0755)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", tmpDirRecovery).Msg("failed to create grub dir")
		return "", err
	}
	_, err = r.config.Fs.Stat(filepath.Join(r.rootfsDir, agentConstants.GrubConf))
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", r.rootfsDir).Msg("failed to stat grub.cfg")
		return "", err
	}
	grubCfg, err := r.config.Fs.ReadFile(filepath.Join(r.rootfsDir, agentConstants.GrubConf))
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", r.rootfsDir).Msg("failed to read grub.cfg")
		return "", err
	}
	err = r.config.Fs.WriteFile(filepath.Join(tmpDirRecovery, agentConstants.GrubConf), grubCfg, 0o644)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", tmpDirRecovery).Msg("failed to write grub.cfg")
		return "", err
	}

	// Now we create an image for the recovery partition
	// We use the dir we created with the image above, which contains the recovery.img and the grub.cfg stuff
	recoverPartitionImage := v1.Image{
		File:       filepath.Join(r.tmpDir, "recovery.img"),
		FS:         agentConstants.LinuxImgFs,
		Label:      agentConstants.RecoveryLabel,
		Size:       uint(size),
		Source:     v1.NewDirSrc(tmpDirRecovery),
		MountPoint: tmpDirRecoveryImage,
	}

	size, _ = config.GetSourceSize(r.config, recoveryImage.Source)
	recoverPartitionImage.Size = uint(size + 100)

	_, err = r.elemental.DeployImageNodirs(&recoverPartitionImage, false)
	// Create recovery.squash from the rootfs into the recovery partition under cOS/
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("source", r.Source).Interface("image", recoverPartitionImage).Msg("failed to create recovery image")
		return "", err
	}

	return recoverPartitionImage.File, nil

}

// CreateEFIPartitionImage creates an EFI partition image with the given source
func (r *RawImage) CreateEFIPartitionImage() (string, error) {
	// Create a temp dir for copying the files to
	tmpDirEfi := filepath.Join(r.tmpDir, "efi")
	err := fsutils.MkdirAll(r.config.Fs, tmpDirEfi, 0755)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", tmpDirEfi).Msg("failed to create temp dir")
		return "", err
	}
	//defer r.config.Fs.RemoveAll(tmpDirEfi)

	// This is where the oem partition will be mounted to copy the files to
	tmpDirEfiMount := filepath.Join(r.tmpDir, "efi-mount")
	err = fsutils.MkdirAll(r.config.Fs, tmpDirEfiMount, 0755)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", tmpDirEfiMount).Msg("failed to create temp dir")
		return "", err
	}
	defer r.config.Fs.RemoveAll(tmpDirEfiMount)

	// Go over the grub folder and copy everything to the image making dirs as needed and going deeper into them
	// Copy the grub.cfg
	// Create dirs as needed
	err = fsutils.MkdirAll(r.config.Fs, filepath.Join(tmpDirEfi, "EFI", "BOOT"), 0755)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", tmpDirEfi).Msg("failed to create boot dir")
		return "", err
	}
	flavor, err := sdkUtils.OSRelease("FLAVOR", filepath.Join(r.rootfsDir, "etc/kairos-release"))
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to get flavor")
		return "", err
	}
	internal.Log.Logger.Debug().Str("flavor", flavor).Msg("got flavor")
	model, err := sdkUtils.OSRelease("MODEL", filepath.Join(r.rootfsDir, "etc/kairos-release"))
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to get model")
		return "", err
	}
	internal.Log.Logger.Debug().Str("model", model).Msg("got model")

	if strings.Contains(flavor, "ubuntu") {
		err = fsutils.MkdirAll(r.config.Fs, filepath.Join(tmpDirEfi, "EFI", "ubuntu"), 0755)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("target", tmpDirEfi).Msg("failed to create ubuntu dir")
			return "", err
		}
		err = r.config.Fs.WriteFile(filepath.Join(tmpDirEfi, "EFI", "ubuntu", "grub.cfg"), []byte(constants.GrubEfiRecovery), 0o644)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("target", tmpDirEfi).Msg("failed to write grub.cfg")
			return "", err
		}
	} else {
		err = r.config.Fs.WriteFile(filepath.Join(tmpDirEfi, "EFI", "BOOT", "grub.cfg"), []byte(constants.GrubEfiRecovery), 0o644)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("target", tmpDirEfi).Msg("failed to write grub.cfg")
			return "", err
		}
	}

	// Now search for the grubARCH.efi and copy it to the efi partition from the rootfs
	if strings.Contains(strings.ToLower(flavor), "alpine") && strings.Contains(strings.ToLower(model), "rpi") {
		internal.Log.Logger.Warn().Msg("Running on Alpine+RPI, not copying shim or grub.")
	} else {
		err = r.copyShimOrGrub(tmpDirEfi, "shim")
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to copy shim")
			return "", err
		}

		err = r.copyShimOrGrub(tmpDirEfi, "grub")
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to copy grub")
			return "", err
		}
	}

	efiPartitionImage := v1.Image{
		File:       filepath.Join(r.tmpDir, "efi.img"),
		FS:         agentConstants.EfiFs,
		Label:      agentConstants.EfiLabel,
		Size:       20,
		Source:     v1.NewDirSrc(tmpDirEfi),
		MountPoint: tmpDirEfiMount,
	}

	// Deploy the source to the image
	_, err = r.elemental.DeployImageNodirs(&efiPartitionImage, false)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("source", r.Source).Interface("image", efiPartitionImage).Msg("failed to create efi image")
		return "", err
	}

	return efiPartitionImage.File, nil
}

func (r *RawImage) Build() error {
	var bootImagePath string
	var err error

	// If we dont have a tempdir, create one
	if r.tmpDir == "" {
		r.tmpDir, err = fsutils.TempDir(r.config.Fs, "", "auroraboot-raw-image-")
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to create temp dir")
			return err
		}
	}
	//defer r.config.Fs.RemoveAll(r.tmpDir)

	// Prepare source
	src, err := v1.NewSrcFromURI(r.Source)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to create source image")
		return err
	}

	// dump the source in a temp dir if its an oci artifact
	// TODO: With the current aurora implementation, this is done in a different step before this, is this needed?
	if src.IsDocker() {
		r.rootfsDir = filepath.Join(r.tmpDir, "rootfs")
		err = fsutils.MkdirAll(r.config.Fs, r.rootfsDir, 0755)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to create temp dir")
			return err
		}
		defer r.config.Fs.RemoveAll(r.rootfsDir)

		e := elemental.NewElemental(r.config)
		_, err = e.DumpSource(r.rootfsDir, src)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to dump source")
			return err
		}
	} else if src.IsDir() {
		r.rootfsDir = src.Value()
	} else {
		internal.Log.Logger.Error().Err(err).Msg("source is not a valid source")
		return err
	}

	// Get the artifact version from the rootfs
	var label string
	if _, ok := r.config.Fs.Stat(filepath.Join(r.rootfsDir, "etc/kairos-release")); ok == nil {
		label, err = sdkUtils.OSRelease("IMAGE_LABEL", filepath.Join(r.rootfsDir, "etc/kairos-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get image label")
			return err
		}
	} else {
		// Before 3.2.x the kairos info was in /etc/os-release
		label, err = sdkUtils.OSRelease("IMAGE_LABEL", filepath.Join(r.rootfsDir, "etc/os-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get image label")
			return err
		}
	}

	// name of isos for example so we store them equally:
	// kairos-ubuntu-24.04-core-amd64-generic-v3.2.4.iso
	outputName := fmt.Sprintf("kairos-%s.raw", label)

	internal.Log.Logger.Info().Msg("Creating BOOT image")
	if r.efi {
		// Create the EFI partition
		bootImagePath, err = r.CreateEFIPartitionImage()
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to create efi partition")
			return err
		}
		defer r.config.Fs.Remove(bootImagePath)
	} else {
		// Create the BIOS partition
		bootImagePath, err = r.CreateBiosPartitionImage()
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to create bios partition")
			return err
		}

		defer r.config.Fs.Remove(bootImagePath)
	}
	internal.Log.Logger.Info().Msg("Created BOOT image")
	internal.Log.Logger.Info().Msg("Creating RECOVERY image")
	// Create the Recovery partition
	recoveryImagePath, err := r.CreateRecoveryPartitionImage()
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to create recovery partition")
		return err
	}
	defer r.config.Fs.Remove(recoveryImagePath)
	internal.Log.Logger.Info().Msg("Created RECOVERY image")

	// Oem after recovery, as it needs the recovery image to calculate the size of the state partition
	internal.Log.Logger.Info().Msg("Creating OEM image")
	// Create the OEM partition
	oemImagePath, err := r.CreateOemPartitionImage(recoveryImagePath)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to create oem partition")
		return err
	}
	defer r.config.Fs.Remove(oemImagePath)
	internal.Log.Logger.Info().Msg("Created OEM image")

	// Create the final disk image
	internal.Log.Logger.Info().Str("target", filepath.Join(r.Output, outputName)).Msg("Assembling final disk image")
	err = r.CreateDiskImage(filepath.Join(r.Output, outputName), []string{bootImagePath, oemImagePath, recoveryImagePath})
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to create disk image")
		return err
	}
	info, err := r.config.Fs.Stat(filepath.Join(r.Output, outputName))
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to stat final image")
		return err
	}
	// truncate the image to desired size
	if r.FinalSize > 0 && uint64(info.Size()) < r.FinalSize*1024*1024 {
		internal.Log.Logger.Info().Int64("size", info.Size()).Msg("Truncating final image")
		err = os.Truncate(filepath.Join(r.Output, outputName), int64(r.FinalSize*1024*1024))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to truncate final image")
			return err
		}

	}
	internal.Log.Logger.Info().Str("target", filepath.Join(r.Output, outputName)).Msg("Assembled final disk image")

	return nil
}

// CreateDiskImage creates the final image by truncating the image with the proper size and
// concatenating the contents of the given partitions.
func (r *RawImage) CreateDiskImage(rawDiskFile string, partImgs []string) error {
	var initDiskFile, endDiskFile string
	var err error
	var partFiles []string
	var size uint64
	var table partition.Table
	var parts []*gpt.Partition

	internal.Log.Logger.Debug().Str("disk", rawDiskFile).Strs("parts", partImgs).Msg("Creating disk image")

	// Create disk image, 1Mb for alignment and GPT header, 2MB for bios boot partition
	// Then concat all partition images
	// Then add 1MB of free space at the end of the disk for gpt backup headers

	// Create the start and end images
	initDiskFile = filepath.Join(r.tmpDir, "init.raw")
	endDiskFile = filepath.Join(r.tmpDir, "end.raw")
	init, err := diskfs.Create(filepath.Join(r.tmpDir, "init.raw"), 3*1024*1024, diskfs.Raw, diskfs.SectorSizeDefault)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", initDiskFile).Msg("failed to create init disk")
		return err
	}
	end, err := diskfs.Create(filepath.Join(r.tmpDir, "end.raw"), 1*1024*1024, diskfs.Raw, diskfs.SectorSize512)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", endDiskFile).Msg("failed to create end disk")
		return err
	}
	err = init.Close()
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", initDiskFile).Msg("failed to close init disk")
		return err
	}
	err = end.Close()
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", endDiskFile).Msg("failed to close end disk")
		return err
	}

	// List and concatenate all image files
	partFiles = append(partFiles, initDiskFile)
	for _, img := range partImgs {
		partFiles = append(partFiles, img)
	}
	partFiles = append(partFiles, endDiskFile)
	err = utils.ConcatFiles(vfs.OSFS, partFiles, rawDiskFile)
	if err != nil {
		return err
	}

	// Add the partition table
	finalDisk, err := diskfs.Open(rawDiskFile)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", rawDiskFile).Msg("failed to open final disk")
		return err
	}
	defer finalDisk.Close()

	// Truncate file to multiple of sector size
	stat, _ := os.Stat(rawDiskFile)
	size = roundToNearestSector(stat.Size(), finalDisk.LogicalBlocksize)
	err = os.Truncate(rawDiskFile, int64(size))
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", rawDiskFile).Msg("failed to truncate final disk")
		return err
	}

	// Create a GPT partition table
	// Size needs to be rounded to the nearest 512 bytes as that's the sector size
	// Leave 1MB at the start for alignment+partition table (so start at sector 2048)
	// 2MB for the BIOS boot partition in case we want to do hybrid boot
	// Which would mean installing grub to the BIOS boot partition
	// BIOS BOOT Legacy, currently does nothing
	size = roundToNearestSector(2*1024*1024, finalDisk.LogicalBlocksize)
	parts = append(parts, &gpt.Partition{
		Start: 2048,
		End:   getSectorEndFromSize(2048, size, finalDisk.LogicalBlocksize),
		Type:  gpt.BIOSBoot,
		Size:  size,
	})
	// EFI
	stat, err = os.Stat(partImgs[0])
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", partImgs[0]).Msg("failed to stat efi partition")
		return err
	}
	size = roundToNearestSector(stat.Size(), finalDisk.LogicalBlocksize)
	parts = append(parts, &gpt.Partition{
		Start: parts[len(parts)-1].End + 1,
		End:   getSectorEndFromSize(parts[len(parts)-1].End+1, size, finalDisk.LogicalBlocksize),
		Type:  gpt.EFISystemPartition,
		Size:  size,
		Name:  agentConstants.EfiPartName,
		GUID:  uuid.NewV5(uuid.NamespaceURL, agentConstants.EfiLabel).String(),
	})
	// OEM
	stat, err = os.Stat(partImgs[1])
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", partImgs[0]).Msg("failed to stat oem partition")
		return err
	}
	size = roundToNearestSector(stat.Size(), finalDisk.LogicalBlocksize)
	parts = append(parts, &gpt.Partition{
		Start: parts[len(parts)-1].End + 1,
		End:   getSectorEndFromSize(parts[len(parts)-1].End+1, size, finalDisk.LogicalBlocksize),
		Type:  gpt.LinuxFilesystem,
		Size:  size,
		Name:  agentConstants.OEMPartName,
		GUID:  uuid.NewV5(uuid.NamespaceURL, agentConstants.OEMLabel).String(),
	})
	// Recovery
	stat, err = os.Stat(partImgs[2])
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", partImgs[0]).Msg("failed to stat recovery partition")
		return err
	}
	size = roundToNearestSector(stat.Size(), finalDisk.LogicalBlocksize)
	parts = append(parts, &gpt.Partition{
		Start: parts[len(parts)-1].End + 1,
		End:   getSectorEndFromSize(parts[len(parts)-1].End+1, size, finalDisk.LogicalBlocksize),
		Type:  gpt.LinuxFilesystem,
		Size:  size,
		Name:  agentConstants.RecoveryImgName,
		GUID:  uuid.NewV5(uuid.NamespaceURL, agentConstants.RecoveryLabel).String(),
	})

	table = &gpt.Table{
		ProtectiveMBR:      true,
		GUID:               agentConstants.DiskUUID, // Set know predictable UUID
		Partitions:         parts,
		LogicalSectorSize:  int(finalDisk.LogicalBlocksize),
		PhysicalSectorSize: int(finalDisk.PhysicalBlocksize),
	}
	err = finalDisk.Partition(table)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", rawDiskFile).Msg("failed to partition final disk")
		return err
	}

	internal.Log.Logger.Info().Str("disk", rawDiskFile).Msg("Created disk image")

	return nil
}

// Helper function to calculate the end sector for a given start and size based on the sector size
func getSectorEndFromSize(start, size uint64, sectorSize int64) uint64 {
	return (size / uint64(sectorSize)) + start - 1
}

// Helper function to round size to the nearest multiple of the sector size
func roundToNearestSector(size, sector int64) uint64 {
	if size%sector == 0 {
		return uint64(size)
	}
	return uint64(size + sector - (size % sector))
}

// CreateBiosPartitionImage TODO: lol
func (r *RawImage) CreateBiosPartitionImage() (string, error) {
	// Would need to format it and then copy the grub files to it
	// Also install grub against it
	// Remember to use the grub artifacts from the rootfs to support secureboot
	return "", nil
}

// copyShim copies the shim/grub file to the EFI partition
// It searches for the shim/grub file in the rootfs and copies it to the EFI partition given in target
func (r *RawImage) copyShimOrGrub(target, which string) error {
	var searchFiles []string
	var copyDone bool
	if which == "shim" {
		searchFiles = sdkUtils.GetEfiShimFiles(runtime.GOARCH)
	} else if which == "grub" {
		searchFiles = sdkUtils.GetEfiGrubFiles(runtime.GOARCH)
	} else {
		return fmt.Errorf("invalid which value: %s", which)
	}
	for _, f := range searchFiles {
		_, err := r.config.Fs.Stat(filepath.Join(r.rootfsDir, f))
		if err != nil {
			r.config.Logger.Debugf("skip copying %s: not found", filepath.Join(r.rootfsDir, f))
			continue
		}
		_, name := filepath.Split(f)
		// remove the .signed suffix if present
		name = strings.TrimSuffix(name, ".signed")
		// remove the .dualsigned suffix if present
		name = strings.TrimSuffix(name, ".dualsigned")
		fileWriteName := filepath.Join(target, fmt.Sprintf("EFI/BOOT/%s", name))
		r.config.Logger.Debugf("Copying %s to %s", f, fileWriteName)

		// Try to find the paths give until we succeed
		fileContent, err := r.config.Fs.ReadFile(filepath.Join(r.rootfsDir, f))

		if err != nil {
			r.config.Logger.Warnf("error reading %s: %s", filepath.Join(r.rootfsDir, f), err)
			continue
		}
		err = r.config.Fs.WriteFile(fileWriteName, fileContent, agentConstants.FilePerm)
		if err != nil {
			return fmt.Errorf("error writing %s: %s", fileWriteName, err)
		}
		copyDone = true

		// Copy the shim content to the fallback name so the system boots from fallback. This means that we do not create
		// any bootloader entries, so our recent installation has the lower priority if something else is on the bootloader
		if which == "shim" {
			writeShim := agentConstants.GetFallBackEfi(runtime.GOARCH)
			err = r.config.Fs.WriteFile(filepath.Join(target, "EFI/BOOT/", writeShim), fileContent, agentConstants.FilePerm)
			if err != nil {
				return fmt.Errorf("could not write file %s at dir %s", writeShim, target)
			}
		}
		break
	}
	if !copyDone {
		r.config.Logger.Debugf("List of files searched for: %s", searchFiles)
		return fmt.Errorf("could not find any shim file to copy")
	}
	return nil
}