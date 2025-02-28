package ops

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

const (
	modelRpi3 = "rpi3"
	modelRpi4 = "rpi4"
	modelRpi5 = "rpi5"
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

type RawImage struct {
	CloudConfig string               // cloud config to copy to the oem partition, if none provided a default one will be created with the kairos user
	Source      string               // Source image to copy the artifacts from, which will be the rootfs in the final image
	Output      string               // Output image destination dir. Final image name will be based on the contents of the source /etc/kairos-release file
	FinalSize   uint64               // Final size of the disk image in MB
	tmpDir      string               // A temp dir to do all work on
	elemental   *elemental.Elemental // Elemental instance to use for the operations
	efi         bool                 // If the image should be EFI or BIOS
	config      *config.Config       // config to use for the operations
}

// NewEFIRawImage creates a new RawImage struct
// config is initialized with a default config to use the standard logger
func NewEFIRawImage(source, output, cc string, finalsize uint64) *RawImage {
	cfg := config.NewConfig(config.WithLogger(internal.Log))
	return &RawImage{efi: true, config: cfg, Source: source, Output: output, elemental: elemental.NewElemental(cfg), CloudConfig: cc, FinalSize: finalsize}
}

func NewBiosRawImage(source, output string, cc string, finalsize uint64) *RawImage {
	cfg := config.NewConfig(config.WithLogger(internal.Log))
	return &RawImage{efi: false, config: cfg, Source: source, Output: output, elemental: elemental.NewElemental(cfg), CloudConfig: cc, FinalSize: finalsize}
}

// createOemPartitionImage creates an OEM partition image with the given cloud config
func (r *RawImage) createOemPartitionImage(recoveryImagePath string) (string, error) {
	// Create a temp dir for copying the files to
	tmpDirOem := filepath.Join(r.TempDir(), "oem")
	err := fsutils.MkdirAll(r.config.Fs, tmpDirOem, 0755)
	defer r.config.Fs.RemoveAll(tmpDirOem)

	// This is where the oem partition will be mounted to copy the files to
	tmpDirOemMount := filepath.Join(r.TempDir(), "oem-mount")
	err = fsutils.MkdirAll(r.config.Fs, tmpDirOemMount, 0755)
	defer r.config.Fs.RemoveAll(tmpDirOemMount)

	// Copy the cloud config to the oem partition if there is any
	ccContent, err := r.config.Fs.ReadFile(r.CloudConfig)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("source", r.CloudConfig).Msg("failed to read cloud config")
		return "", err
	}
	if r.CloudConfig != "" && len(ccContent) > 0 {
		internal.Log.Logger.Debug().Str("source", r.CloudConfig).Str("target", filepath.Join(tmpDirOem, "90_custom.yaml")).Msg("Copying cloud config to oem partition")
		f, err := r.config.Fs.ReadFile(r.CloudConfig)
		if err != nil {
			return "", err
		}
		internal.Log.Logger.Debug().Str("source", r.CloudConfig).Str("target", filepath.Join(tmpDirOem, "90_custom.yaml")).Str("content", string(f)).Interface("s", f).Msg("Copying cloud config to oem partition")
		err = fsutils.Copy(r.config.Fs, r.CloudConfig, filepath.Join(tmpDirOem, "90_custom.yaml"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("source", r.CloudConfig).Str("target", filepath.Join(tmpDirOem, "90_custom.yaml")).Msg("failed to copy cloud config")
			return "", err
		}
	} else {
		// Create a default cloud config yaml with at least a user
		internal.Log.Logger.Debug().Str("target", filepath.Join(tmpDirOem, "90_custom.yaml")).Msg("Creating default cloud config")
		err = r.config.Fs.WriteFile(filepath.Join(tmpDirOem, "90_custom.yaml"), []byte(constants.DefaultCloudConfig), 0o644)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("target", filepath.Join(tmpDirOem, "90_custom.yaml")).Msg("failed to write cloud config")
			return "", err
		}
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
								FileSystem: agentConstants.LinuxFs,
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
								FileSystem: agentConstants.LinuxFs,
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
	internal.Log.Logger.Debug().Str("target", filepath.Join(tmpDirOem, resetCloudInit)).Msg("Creating reset cloud config")
	err = r.config.Fs.WriteFile(filepath.Join(tmpDirOem, resetCloudInit), yipYAML, 0o644)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", filepath.Join(tmpDirOem, resetCloudInit)).Msg("failed to write cloud config")
		return "", err
	}

	OemPartitionImage := v1.Image{
		File:       filepath.Join(r.TempDir(), "oem.img"),
		FS:         agentConstants.LinuxFs,
		Label:      agentConstants.OEMLabel,
		Size:       agentConstants.OEMSize,
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

// createRecoveryPartitionImage creates a recovery partition image with the given source
// The source expects to be a directory with the rootfs to generate a squashfs from
// This generates a recovery.img with the rootfs on in under /cOS/
// It also contains the final grub.cfg and grubenv_first
func (r *RawImage) createRecoveryPartitionImage() (string, error) {
	// Create a temp dir for mounting the image to
	tmpDirRecoveryImage := filepath.Join(r.TempDir(), "recovery-img")
	err := fsutils.MkdirAll(r.config.Fs, tmpDirRecoveryImage, 0755)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", tmpDirRecoveryImage).Msg("failed to create temp dir")
		return "", err
	}
	defer r.config.Fs.RemoveAll(tmpDirRecoveryImage)

	// Create a dir to store the recovery partition contents
	tmpDirRecovery := filepath.Join(r.TempDir(), "recovery")
	err = fsutils.MkdirAll(r.config.Fs, tmpDirRecovery, 0755)
	defer r.config.Fs.RemoveAll(tmpDirRecovery)

	err = fsutils.MkdirAll(r.config.Fs, filepath.Join(tmpDirRecovery, "cOS"), 0755)

	recoveryImage := &v1.Image{
		File:       filepath.Join(tmpDirRecovery, "cOS", agentConstants.RecoveryImgFile),
		FS:         agentConstants.LinuxImgFs,
		Label:      agentConstants.SystemLabel,
		Source:     v1.NewDirSrc(r.Source),
		MountPoint: tmpDirRecoveryImage,
	}
	size, _ := config.GetSourceSize(r.config, recoveryImage.Source)
	// Add some extra space to the image in case the calculation is a bit off
	recoveryImage.Size = uint(size + 100)

	_, err = r.elemental.DeployImage(recoveryImage, false)
	// Create recovery.squash from the rootfs into the recovery partition under cOS/
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("source", r.Source).Interface("image", recoveryImage).Msg("failed to create recovery image")
		return "", err
	}

	// TODO: Copy the grub artifacts to the recovery partition under grub2/ so they can be used by the EFI/BIOS grub
	// contents come form https://github.com/kairos-io/packages/blob/main/packages/static/grub-config/files/grub.cfg

	// Copy the grub.cfg from the rootfs into the recovery partition
	internal.Log.Logger.Debug().Str("source", r.Source).Str("target", filepath.Join(tmpDirRecovery, filepath.Dir(agentConstants.GrubConf))).Msg("Copying grub.cfg")
	err = fsutils.MkdirAll(r.config.Fs, filepath.Join(tmpDirRecovery, filepath.Dir(agentConstants.GrubConf)), 0755)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", tmpDirRecovery).Msg("failed to create grub dir")
		return "", err
	}
	_, err = r.config.Fs.Stat(filepath.Join(r.Source, agentConstants.GrubConf))
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", r.Source).Msg("failed to stat grub.cfg")
		return "", err
	}
	grubCfg, err := r.config.Fs.ReadFile(filepath.Join(r.Source, agentConstants.GrubConf))
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", r.Source).Msg("failed to read grub.cfg")
		return "", err
	}
	err = r.config.Fs.WriteFile(filepath.Join(tmpDirRecovery, agentConstants.GrubConf), grubCfg, 0o644)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", tmpDirRecovery).Msg("failed to write grub.cfg")
		return "", err
	}

	// Now we create an image for the recovery partition
	// We use the dir we created with the image above, which contains the recovery.img and the grub.cfg stuff
	recoverPartitionImage := &v1.Image{
		File:       filepath.Join(r.TempDir(), "recovery.img"),
		FS:         agentConstants.LinuxFs,
		Label:      agentConstants.RecoveryLabel,
		Size:       uint(size),
		Source:     v1.NewDirSrc(tmpDirRecovery),
		MountPoint: tmpDirRecoveryImage,
	}

	size, _ = config.GetSourceSize(r.config, recoveryImage.Source)
	// Add some extra space to the image in case the calculation is a bit off
	// we add an extra 50Mb of top as the recovery.img has to fit in there plus any artifacts we copy
	// Double the size as the partition needs to account for recovery and transition image during recovery upgrade
	recoverPartitionImage.Size = uint(size*2 + 150)

	_, err = r.elemental.DeployImageNodirs(recoverPartitionImage, false)
	// Create recovery.squash from the rootfs into the recovery partition under cOS/
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("source", r.Source).Interface("image", recoverPartitionImage).Msg("failed to create recovery image")
		return "", err
	}

	return recoverPartitionImage.File, nil

}

// createEFIPartitionImage creates an EFI partition image with the given source
func (r *RawImage) createEFIPartitionImage() (string, error) {
	// Create a temp dir for copying the files to
	tmpDirEfi := filepath.Join(r.TempDir(), "efi")
	err := fsutils.MkdirAll(r.config.Fs, tmpDirEfi, 0755)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", tmpDirEfi).Msg("failed to create temp dir")
		return "", err
	}
	defer r.config.Fs.RemoveAll(tmpDirEfi)

	// This is where the oem partition will be mounted to copy the files to
	tmpDirEfiMount := filepath.Join(r.TempDir(), "efi-mount")
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

	model, flavor, err := r.GetModelAndFlavor()
	internal.Log.Logger.Info().Str("model", model).Str("flavor", flavor).Msg("model and flavor")

	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to get flavor or model")
		return "", err
	}

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

	// Do board specific stuff
	if model == modelRpi4 || model == modelRpi5 {
		err = copyFirmwareRpi(tmpDirEfi, model)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to copy rpi firmware")
			return "", err
		}
	}

	efiPartitionImage := v1.Image{
		File:       filepath.Join(r.TempDir(), "efi.img"),
		FS:         agentConstants.EfiFs,
		Label:      agentConstants.EfiLabel,
		Size:       agentConstants.EfiSize,
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

// createBiosPartitionImage creates a BIOS partition image
// This is an empty image as grub-install will then write the core.img contents to it directly so nothing special.
func (r *RawImage) createBiosPartitionImage() (string, error) {
	f, err := r.config.Fs.Create(filepath.Join(r.TempDir(), "bios.img"))
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", filepath.Join(r.TempDir(), "bios.img")).Msg("failed to create bios image")
		return "", err
	}
	err = f.Truncate(int64(agentConstants.BiosSize * 1024 * 1024))
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", filepath.Join(r.TempDir(), "bios.img")).Msg("failed to truncate bios image")
		return "", err
	}
	err = f.Close()
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", filepath.Join(r.TempDir(), "bios.img")).Msg("failed to close bios image")
		return "", err
	}

	return filepath.Join(r.TempDir(), "bios.img"), err
}

func (r *RawImage) TempDir() string {
	if r.tmpDir == "" {
		r.tmpDir, _ = fsutils.TempDir(r.config.Fs, "", "auroraboot-raw-image-")
	}
	return r.tmpDir
}

func (r *RawImage) Build() error {
	var bootImagePath string
	var err error

	defer r.config.Fs.RemoveAll(r.TempDir())

	// Get the artifact version from the rootfs
	var label string
	var flavor string
	if _, ok := r.config.Fs.Stat(filepath.Join(r.Source, "etc/kairos-release")); ok == nil {
		label, err = sdkUtils.OSRelease("IMAGE_LABEL", filepath.Join(r.Source, "etc/kairos-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get image label")
			return err
		}
		flavor, err = sdkUtils.OSRelease("FLAVOR", filepath.Join(r.Source, "etc/kairos-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get image flavor")
			return err
		}
	} else {
		// Before 3.2.x the kairos info was in /etc/os-release
		flavor, err = sdkUtils.OSRelease("FLAVOR", filepath.Join(r.Source, "etc/os-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get image label")
			return err
		}
		label, err = sdkUtils.OSRelease("IMAGE_LABEL", filepath.Join(r.Source, "etc/os-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get image label")
			return err
		}
	}

	// name of isos for example so we store them equally:
	// kairos-ubuntu-24.04-core-amd64-generic-v3.2.4.iso
	outputName := fmt.Sprintf("kairos-%s-%s.raw", flavor, label)
	internal.Log.Logger.Debug().Str("name", outputName).Msg("Got output name")

	internal.Log.Logger.Info().Msg("Creating RECOVERY image")
	// Create the Recovery partition
	recoveryImagePath, err := r.createRecoveryPartitionImage()
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to create recovery partition")
		return err
	}
	defer r.config.Fs.Remove(recoveryImagePath)
	internal.Log.Logger.Info().Msg("Created RECOVERY image")

	internal.Log.Logger.Info().Msg("Creating BOOT image")
	if r.efi {
		// Create the EFI partition
		bootImagePath, err = r.createEFIPartitionImage()
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to create efi partition")
			return err
		}
		defer r.config.Fs.Remove(bootImagePath)
	} else {
		// Create the BIOS partition AFTER the recovery partition, as it needs the recovery image to install grub
		bootImagePath, err = r.createBiosPartitionImage()
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to create bios partition")
			return err
		}

		defer r.config.Fs.Remove(bootImagePath)
	}
	internal.Log.Logger.Info().Msg("Created BOOT image")

	// Oem after recovery, as it needs the recovery image to calculate the size of the state partition
	internal.Log.Logger.Info().Msg("Creating OEM image")
	// Create the OEM partition
	oemImagePath, err := r.createOemPartitionImage(recoveryImagePath)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to create oem partition")
		return err
	}
	defer r.config.Fs.Remove(oemImagePath)
	internal.Log.Logger.Info().Msg("Created OEM image")

	// Create the final disk image
	internal.Log.Logger.Info().Str("target", filepath.Join(r.Output, outputName)).Msg("Assembling final disk image")
	err = r.createDiskImage(filepath.Join(r.Output, outputName), []string{bootImagePath, oemImagePath, recoveryImagePath})
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

	// Do some final adjustments for boards
	err = r.FinalizeImage(filepath.Join(r.Output, outputName))
	internal.Log.Logger.Info().Str("target", filepath.Join(r.Output, outputName)).Msg("Assembled final disk image")

	return nil
}

// createDiskImage creates the final image by truncating the image with the proper size and
// concatenating the contents of the given partitions.
func (r *RawImage) createDiskImage(rawDiskFile string, partImgs []string) error {
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
	initDiskFile = filepath.Join(r.TempDir(), "init.raw")
	endDiskFile = filepath.Join(r.TempDir(), "end.raw")
	// THIS SIZE MARKS THE START SECTOR FOR THE PARTITIONS BELOW!
	// So this mean we have an empty 2048 sectors at the start of the disk, partitions then start at that point
	init, err := diskfs.Create(filepath.Join(r.TempDir(), "init.raw"), 1*1024*1024, diskfs.Raw, diskfs.SectorSizeDefault)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("target", initDiskFile).Msg("failed to create init disk")
		return err
	}
	end, err := diskfs.Create(filepath.Join(r.TempDir(), "end.raw"), 1*1024*1024, diskfs.Raw, diskfs.SectorSize512)
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
	// Bit 0: System partition This indicates the system that this is a required partition and to not mess with it.
	// Bit 2: Legacy BIOS bootable This indicates that this partition is bootable by legacy BIOS.
	if r.efi {
		// EFI
		stat, err = os.Stat(partImgs[0])
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("target", partImgs[0]).Msg("failed to stat efi partition")
			return err
		}
		size = roundToNearestSector(stat.Size(), finalDisk.LogicalBlocksize)
		parts = append(parts, &gpt.Partition{
			Start:      2048,
			End:        getSectorEndFromSize(2048, size, finalDisk.LogicalBlocksize),
			Type:       gpt.EFISystemPartition,
			Size:       size,
			Name:       agentConstants.EfiPartName,
			GUID:       uuid.NewV5(uuid.NamespaceURL, agentConstants.EfiLabel).String(),
			Attributes: 1 << 0, // Sets bit 0
		})
	} else {
		size = roundToNearestSector(int64(agentConstants.BiosSize*1024*1024), finalDisk.LogicalBlocksize)
		parts = append(parts, &gpt.Partition{
			Start:      2048,
			End:        getSectorEndFromSize(2048, size, finalDisk.LogicalBlocksize),
			Type:       gpt.BIOSBoot,
			Size:       size,
			Name:       agentConstants.BiosPartName,
			GUID:       uuid.NewV5(uuid.NamespaceURL, agentConstants.EfiLabel).String(), // Same name as EFI, COS_GRUB usually
			Attributes: (1 << 0) | (1 << 2),                                             // Sets bits 0 and 2
		})
	}

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

	// If its not efi, we need to install grub to the disk device directly
	if !r.efi {
		err = r.installGrubToDisk(rawDiskFile)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("target", rawDiskFile).Msg("failed to install grub to final disk")
			return err
		}
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

// copyShim copies the shim/grub file to the EFI partition
// It searches for the shim/grub file in the rootfs and copies it to the EFI partition given in target
func (r *RawImage) copyShimOrGrub(target, which string) error {
	var searchFiles []string
	var copyDone bool
	var arch string

	// Try to get the arch from the source rootfs
	arch = runtime.GOARCH
	parsedArch, err := sdkUtils.OSRelease("KAIROS_TARGETARCH", filepath.Join(r.Source, "etc/kairos-release"))
	if err == nil && parsedArch != "" {
		arch = parsedArch
	} else {
		internal.Log.Logger.Warn().Err(err).Str("arch", runtime.GOARCH).Msg("failed to geta arch from source rootfs, defaulting to use artifacts from runtime arch")
	}

	if which == "shim" {
		searchFiles = sdkUtils.GetEfiShimFiles(arch)
	} else if which == "grub" {
		searchFiles = sdkUtils.GetEfiGrubFiles(arch)
	} else {
		return fmt.Errorf("invalid which value: %s", which)
	}

	for _, f := range searchFiles {
		_, err := r.config.Fs.Stat(filepath.Join(r.Source, f))
		if err != nil {
			r.config.Logger.Debugf("skip copying %s: not found", filepath.Join(r.Source, f))
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
		fileContent, err := r.config.Fs.ReadFile(filepath.Join(r.Source, f))

		if err != nil {
			r.config.Logger.Warnf("error reading %s: %s", filepath.Join(r.Source, f), err)
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
			writeShim := agentConstants.GetFallBackEfi(arch)
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

func (r *RawImage) installGrubToDisk(image string) error {
	internal.Log.Logger.Debug().Str("backingFile", image).Msg("Attaching file to loop device")
	// Create a dir to store the recovery partition contents
	tmpDirRecovery := filepath.Join(r.TempDir(), "recovery")
	err := fsutils.MkdirAll(r.config.Fs, tmpDirRecovery, 0755)
	defer r.config.Fs.RemoveAll(tmpDirRecovery)

	// TODO: move this to a function
	// TODO hardcore edition: Make this working pure golang
	// doable but complex:
	// mount device into a loop device via syscalls
	// refresh the partition table
	// create the disk nodes to map to the partitions
	// then do it in reverse to cleanup
	// I tried but the devices didnt appear properly dur to it being in a container
	// so I went with the easy way
	out, err := exec.Command("losetup", "-D").CombinedOutput()
	internal.Log.Logger.Debug().Str("output", string(out)).Msg("Detaching loop devices")
	if err != nil {
		return fmt.Errorf("failed to detach loop devices: %w", err)
	}

	loopDevice, err := exec.Command("losetup", "-f", "--show", image).CombinedOutput()
	internal.Log.Logger.Debug().Str("output", string(loopDevice)).Msg("Attaching image to loop device")
	if err != nil {
		return fmt.Errorf("failed to attach file to loop device: %w", err)
	}

	// clean loop device, trim spaces and such
	loopDevice = bytes.TrimSpace(loopDevice)

	// Run kpartx
	out, err = exec.Command("kpartx", "-av", string(loopDevice)).CombinedOutput()
	internal.Log.Logger.Debug().Str("output", string(out)).Msg("Running kpartx")
	if err != nil {
		internal.Log.Logger.Error().Str("output", string(out)).Msg("kpartx output")
		return fmt.Errorf("failed to run kpartx: %w", err)
	}

	defer func() {
		// TODO: move this to a function
		out, err := exec.Command("kpartx", "-dv", string(loopDevice)).CombinedOutput()
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("device", string(loopDevice)).Str("out", string(out)).Msg("failed to detach loop device")
			return
		}
		out, err = exec.Command("losetup", "-d", string(loopDevice)).CombinedOutput()
		if err != nil {
			internal.Log.Logger.Error().Err(err).Str("device", string(loopDevice)).Str("out", string(out)).Msg("failed to detach loop device")
			return
		}
	}()

	internal.Log.Logger.Debug().Str("device", string(loopDevice)).Str("image", image).Msg("Attached image to loop device")

	// While grub is installed to disk + bios_boot partition, it still needs to have the config and mod files available
	// so, the grub files are stored in the recovery partition
	// Get only the loop device without the /dev/ prefix
	cleanLoopDevice := string(loopDevice)[5:]
	recoveryLoop := fmt.Sprintf("/dev/mapper/%s%s", cleanLoopDevice, "p3")
	err = unix.Mount(recoveryLoop, tmpDirRecovery, agentConstants.LinuxFs, 0, "")
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("device", recoveryLoop).Str("mountpoint", tmpDirRecovery).Msg("failed to mount recovery partition")
		return err
	}

	defer unix.Unmount(tmpDirRecovery, 0)

	// Unfortunately this is the only way to install grub to a disk
	// I tried to manually do it but grub does way too much stuff
	// If someone is braver than me, here are some tips.
	// boot.img goes to the first sector of the disk, sector 0 (512 bytes)
	// core.img goes to the partition identified by bios_grub flag
	// boot.img needs to have the LBA where core.img is located
	// No idea where is that hardcoded
	// core.img is not provided by grub packages, it has to be generated beforehand
	args := []string{
		"--target=i386-pc",
		"--force", string(loopDevice),
		fmt.Sprintf("--boot-directory=%s", tmpDirRecovery),
	}
	internal.Log.Logger.Debug().Strs("args", args).Msg("Running grub2-install")
	out, err = exec.Command("grub2-install", args...).CombinedOutput()
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("device", string(loopDevice)).Str("output", string(out)).Msg("failed to install grub")
		return err
	}
	// Copy recovery grub.cfg to /mnt/recovery/grub2/grub.cfg so its picked up by the grub which then chainloads the rest
	err = r.config.Fs.WriteFile(filepath.Join(tmpDirRecovery, "grub2", "grub.cfg"), []byte(constants.GrubEfiRecovery), 0o644)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Str("device", string(loopDevice)).Msg("failed to write grub.cfg")
		return err
	}

	return nil
}

// GetModelAndFlavor returns the model and flavor of the source rootfs
func (r *RawImage) GetModelAndFlavor() (string, string, error) {
	var flavor string
	var model string
	var err error

	if _, ok := r.config.Fs.Stat(filepath.Join(r.Source, "etc/kairos-release")); ok == nil {
		flavor, err = sdkUtils.OSRelease("FLAVOR", filepath.Join(r.Source, "etc/kairos-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get flavor")
			return "", "", err
		}
		model, err = sdkUtils.OSRelease("MODEL", filepath.Join(r.Source, "etc/kairos-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get model")
			return "", "", err
		}
	} else {
		// Fallback to /etc/os-release for older images
		flavor, err = sdkUtils.OSRelease("FLAVOR", filepath.Join(r.Source, "etc/os-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get flavor")
			return "", "", err
		}
		model, err = sdkUtils.OSRelease("MODEL", filepath.Join(r.Source, "etc/os-release"))
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to get model")
			return "", "", err
		}
	}

	if flavor == "" || model == "" {
		internal.Log.Logger.Error().Msg("failed to get flavor or model")
		return "", "", fmt.Errorf("failed to get flavor or model")
	}

	internal.Log.Logger.Debug().Str("flavor", flavor).Msg("got flavor")
	internal.Log.Logger.Debug().Str("model", model).Msg("got model")

	return model, flavor, nil
}

// FinalizeImage does some final adjustments to the image
func (r *RawImage) FinalizeImage(image string) error {
	var err error

	// Get the model
	model, _, err := r.GetModelAndFlavor()
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to get flavor or model")
		return err
	}

	// Do board specific stuff
	switch model {
	case modelRpi5, modelRpi4, modelRpi3:
		internal.Log.Logger.Debug().Str("model", model).Msg("Running on RPI.")
	case "odroid-c2":
		internal.Log.Logger.Debug().Str("model", model).Msg("Running on Odroid-C2.")
		err = utils.DD("/firmware/odroid-c2/bl1.bin.hardkernel", image, 1, 442, 0, 0)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to dd bl1.bin.hardkernel")
			return err
		}
		err = utils.DD("/firmware/odroid-c2/bl1.bin.hardkernel", image, 512, 0, 1, 1)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to dd bl1.bin.hardkernel")
			return err
		}
		err = utils.DD("/firmware/odroid-c2/u-boot.odroidc2", image, 512, 0, 0, 97)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to dd u-boot.odroidc2")
			return err
		}
	case "pinebookpro":
		internal.Log.Logger.Debug().Str("model", model).Msg("Running on Pinebook Pro.")
		err = utils.DD("/pinebookpro/u-boot/usr/lib/u-boot/pinebook-pro-rk3399/idbloader.img", image, 64, 0, 0, 0)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to dd idbloader.img")
			return err
		}
		err = utils.DD("/pinebookpro/u-boot/usr/lib/u-boot/pinebook-pro-rk3399/u-boot.itb", image, 16384, 0, 0, 0)
		if err != nil {
			internal.Log.Logger.Error().Err(err).Msg("failed to dd u-boot.itb")
			return err
		}
	}

	// Set the final image to be used by all as we run inside a container and the image is owned by root otherwise
	err = r.config.Fs.Chmod(image, 0777)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("failed to chmod final image")
		return err
	}
	return nil
}
