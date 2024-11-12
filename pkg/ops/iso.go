package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/constants"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/AuroraBoot/pkg/utils"
	"github.com/otiai10/copy"
	"github.com/twpayne/go-vfs/v4"

	"github.com/kairos-io/kairos-agent/v2/pkg/cloudinit"
	agentconfig "github.com/kairos-io/kairos-agent/v2/pkg/config"
	"github.com/kairos-io/kairos-agent/v2/pkg/elemental"
	"github.com/kairos-io/kairos-agent/v2/pkg/http"
	v1types "github.com/kairos-io/kairos-agent/v2/pkg/types/v1"
	sdkTypes "github.com/kairos-io/kairos-sdk/types"
	sdkutils "github.com/kairos-io/kairos-sdk/utils"
	"github.com/sanity-io/litter"
)

type LiveISO struct {
	RootFS             []*v1types.ImageSource `yaml:"rootfs,omitempty" mapstructure:"rootfs"`
	UEFI               []*v1types.ImageSource `yaml:"uefi,omitempty" mapstructure:"uefi"`
	Image              []*v1types.ImageSource `yaml:"image,omitempty" mapstructure:"image"`
	Label              string                 `yaml:"label,omitempty" mapstructure:"label"`
	GrubEntry          string                 `yaml:"grub-entry-name,omitempty" mapstructure:"grub-entry-name"`
	BootloaderInRootFs bool                   `yaml:"bootloader-in-rootfs" mapstructure:"bootloader-in-rootfs"`
}

// BuildConfig represents the config we need for building isos, raw images, artifacts
type BuildConfig struct {
	Date   bool   `yaml:"date,omitempty" mapstructure:"date"`
	Name   string `yaml:"name,omitempty" mapstructure:"name"`
	OutDir string `yaml:"output,omitempty" mapstructure:"output"`

	// 'inline' and 'squash' labels ensure config fields
	// are embedded from a yaml and map PoV
	agentconfig.Config `yaml:",inline" mapstructure:",squash"`
}

type BuildISOAction struct {
	cfg  *BuildConfig
	spec *LiveISO
	e    *elemental.Elemental
}

type BuildISOActionOption func(a *BuildISOAction)
type GenericOptions func(a *agentconfig.Config) error

func NewBuildConfig(opts ...GenericOptions) *BuildConfig {
	b := &BuildConfig{
		Config: *NewConfig(opts...),
		Name:   constants.BuildImgName,
	}
	return b
}

func NewConfig(opts ...GenericOptions) *agentconfig.Config {
	log := sdkTypes.NewKairosLogger("auroraboot", "info", false)
	arch, err := utils.GolangArchToArch(runtime.GOARCH)
	if err != nil {
		log.Errorf("invalid arch: %s", err.Error())
		return nil
	}

	c := &agentconfig.Config{
		Fs:                    vfs.OSFS,
		Logger:                log,
		Syscall:               &v1types.RealSyscall{},
		Client:                http.NewClient(),
		Arch:                  arch,
		SquashFsNoCompression: true,
	}
	for _, o := range opts {
		err := o(c)
		if err != nil {
			log.Errorf("error applying config option: %s", err.Error())
			return nil
		}
	}

	// delay runner creation after we have run over the options in case we use WithRunner
	if c.Runner == nil {
		c.Runner = &v1types.RealRunner{Logger: &c.Logger}
	}

	// Now check if the runner has a logger inside, otherwise point our logger into it
	// This can happen if we set the WithRunner option as that doesn't set a logger
	if c.Runner.GetLogger() == nil {
		c.Runner.SetLogger(&c.Logger)
	}

	// Delay the yip runner creation, so we set the proper logger instead of blindly setting it to the logger we create
	// at the start of NewRunConfig, as WithLogger can be passed on init, and that would result in 2 different logger
	// instances, on the config.Logger and the other on config.CloudInitRunner
	if c.CloudInitRunner == nil {
		c.CloudInitRunner = cloudinit.NewYipCloudInitRunner(c.Logger, c.Runner, vfs.OSFS)
	}
	litter.Config.HidePrivateFields = false

	return c
}

// GenISO generates an ISO from a rootfs, and stores results in dst
func GenISO(src, dst string, i schema.ISO) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "geniso")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		overlay := tmp
		if i.DataPath != "" {
			overlay = i.DataPath
		}

		// We are assuming StepCopyCloudConfig has already run, putting it the config in "dst"
		err = copyFileIfExists(filepath.Join(dst, "config.yaml"), filepath.Join(overlay, "config.yaml"))
		if err != nil {
			return err
		}

		internal.Log.Logger.Info().Msgf("Generating iso '%s' from '%s' to '%s'", i.Name, src, dst)
		cfg := NewBuildConfig(
			WithLogger(sdkTypes.NewKairosLogger("auroraboot", "debug", false)),
		)
		cfg.Name = i.Name
		cfg.OutDir = dst
		cfg.Date = i.IncludeDate
		if i.Arch != "" {
			cfg.Arch = i.Arch
		}

		spec := &LiveISO{
			RootFS:             []*v1types.ImageSource{v1types.NewDirSrc(src)},
			Image:              []*v1types.ImageSource{v1types.NewDirSrc("/grub2"), v1types.NewDirSrc(overlay)},
			Label:              constants.ISOLabel,
			GrubEntry:          "Kairos",
			BootloaderInRootFs: false,
		}

		if i.OverlayRootfs != "" {
			spec.RootFS = append(spec.RootFS, v1types.NewDirSrc(i.OverlayRootfs))
		}
		if i.OverlayUEFI != "" {
			spec.UEFI = append(spec.UEFI, v1types.NewDirSrc(i.OverlayUEFI))
		}
		if i.OverlayISO != "" {
			spec.Image = append(spec.Image, v1types.NewDirSrc(i.OverlayISO))
		}

		buildISO := NewBuildISOAction(cfg, spec)
		err = buildISO.ISORun()
		if err != nil {
			internal.Log.Logger.Error().Msgf("Failed generating iso '%s' from '%s'. Error: %s", i.Name, src, err.Error())
		}
		return err
	}
}

func InjectISO(dst, isoFile string, i schema.ISO) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		os.Chdir(dst)
		injectedIso := isoFile + ".custom.iso"
		os.Remove(injectedIso)

		tmp, err := os.MkdirTemp("", "injectiso")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		if i.DataPath != "" {
			internal.Log.Logger.Info().Msgf("Adding data in '%s' to '%s'", i.DataPath, isoFile)
			err = copy.Copy(i.DataPath, tmp)
			if err != nil {
				return err
			}
		}

		internal.Log.Logger.Info().Msgf("Adding cloud config file to '%s'", isoFile)
		err = copy.Copy(filepath.Join(dst, "config.yaml"), filepath.Join(tmp, "config.yaml"))
		if err != nil {
			return err
		}

		out, err := sdkutils.SH(fmt.Sprintf("xorriso -indev %s -outdev %s -map %s / -boot_image any replay", isoFile, injectedIso, tmp))
		internal.Log.Print(out)
		if err != nil {
			return err
		}
		internal.Log.Logger.Info().Msgf("Wrote '%s'", injectedIso)
		return err
	}
}

func copyFileIfExists(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		} else {
			return err
		}
	}
	return copy.Copy(src, dst)
}

func NewBuildISOAction(cfg *BuildConfig, spec *LiveISO, opts ...BuildISOActionOption) *BuildISOAction {
	b := &BuildISOAction{
		cfg:  cfg,
		e:    elemental.NewElemental(&cfg.Config),
		spec: spec,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// ISORun will install the system from a given configuration
func (b *BuildISOAction) ISORun() (err error) {
	cleanup := sdkutils.NewCleanStack()
	defer func() { err = cleanup.Cleanup(err) }()

	isoTmpDir, err := utils.TempDir(b.cfg.Fs, "", "auroraboot-iso")
	if err != nil {
		return err
	}
	cleanup.Push(func() error { return b.cfg.Fs.RemoveAll(isoTmpDir) })

	rootDir := filepath.Join(isoTmpDir, "rootfs")
	err = utils.MkdirAll(b.cfg.Fs, rootDir, constants.DirPerm)
	if err != nil {
		return err
	}

	uefiDir := filepath.Join(isoTmpDir, "uefi")
	err = utils.MkdirAll(b.cfg.Fs, uefiDir, constants.DirPerm)
	if err != nil {
		return err
	}

	isoDir := filepath.Join(isoTmpDir, "iso")
	err = utils.MkdirAll(b.cfg.Fs, isoDir, constants.DirPerm)
	if err != nil {
		return err
	}

	if b.cfg.OutDir != "" {
		err = utils.MkdirAll(b.cfg.Fs, b.cfg.OutDir, constants.DirPerm)
		if err != nil {
			b.cfg.Logger.Errorf("Failed creating output folder: %s", b.cfg.OutDir)
			return err
		}
	}

	b.cfg.Logger.Infof("Preparing squashfs root...")
	err = b.applySources(rootDir, b.spec.RootFS...)
	if err != nil {
		b.cfg.Logger.Errorf("Failed installing OS packages: %v", err)
		return err
	}
	err = utils.CreateDirStructure(b.cfg.Fs, rootDir)
	if err != nil {
		b.cfg.Logger.Errorf("Failed creating root directory structure: %v", err)
		return err
	}

	b.cfg.Logger.Infof("Preparing ISO image root tree...")
	err = b.applySources(isoDir, b.spec.Image...)
	if err != nil {
		b.cfg.Logger.Errorf("Failed installing ISO image packages: %v", err)
		return err
	}

	err = b.prepareISORoot(isoDir, rootDir)
	if err != nil {
		b.cfg.Logger.Errorf("Failed preparing ISO's root tree: %v", err)
		return err
	}

	err = b.prepareBootArtifacts(isoDir)
	if err != nil {
		b.cfg.Logger.Errorf("Failed preparing boot artifacts: %v", err)
		return err
	}

	b.cfg.Logger.Infof("Creating ISO image...")
	err = b.burnISO(isoDir)
	if err != nil {
		b.cfg.Logger.Errorf("Failed creating ISO image: %v", err)
		return err
	}

	return err
}

// prepareBootArtifacts will write the needed artifacts for BIOS cd boot into the isoDir
// so xorriso can use those to build the bootable iso file
func (b *BuildISOAction) prepareBootArtifacts(isoDir string) error {
	err := os.WriteFile(filepath.Join(isoDir, constants.IsoBootFile), constants.Eltorito, constants.FilePerm)
	if err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(isoDir, constants.IsoHybridMBR), constants.BootHybrid, constants.FilePerm)
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Join(isoDir, constants.GrubPrefixDir), constants.DirPerm)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(isoDir, constants.GrubPrefixDir, constants.GrubCfg), constants.GrubLiveBiosCfg, constants.FilePerm)
}

func (b BuildISOAction) prepareISORoot(isoDir string, rootDir string) error {
	kernel, initrd, err := b.e.FindKernelInitrd(rootDir)
	if err != nil {
		b.cfg.Logger.Error("Could not find kernel and/or initrd")
		return err
	}
	err = utils.MkdirAll(b.cfg.Fs, filepath.Join(isoDir, "boot"), constants.DirPerm)
	if err != nil {
		return err
	}
	//TODO document boot/kernel and boot/initrd expectation in bootloader config
	b.cfg.Logger.Debugf("Copying Kernel file %s to iso root tree", kernel)
	err = utils.CopyFile(b.cfg.Fs, kernel, filepath.Join(isoDir, constants.IsoKernelPath))
	if err != nil {
		return err
	}

	b.cfg.Logger.Debugf("Copying initrd file %s to iso root tree", initrd)
	err = utils.CopyFile(b.cfg.Fs, initrd, filepath.Join(isoDir, constants.IsoInitrdPath))
	if err != nil {
		return err
	}

	b.cfg.Logger.Info("Creating EFI image...")
	err = b.createEFI(rootDir, isoDir)
	if err != nil {
		return err
	}

	b.cfg.Logger.Info("Creating squashfs...")
	err = utils.CreateSquashFS(b.cfg.Runner, b.cfg.Logger, rootDir, filepath.Join(isoDir, constants.IsoRootFile), constants.GetDefaultSquashfsOptions())
	if err != nil {
		return err
	}

	return nil
}

// createEFI creates the EFI image that is used for booting
// it searches the rootfs for the shim/grub.efi file and copies it into a directory with the proper EFI structure
// then it generates a grub.cfg that chainloads into the grub.cfg of the livecd (which is the normal livecd grub config from luet packages)
// then it calculates the size of the EFI image based on the files copied and creates the image
func (b BuildISOAction) createEFI(rootdir string, isoDir string) error {
	var err error

	// rootfs /efi dir
	img := filepath.Join(isoDir, constants.IsoEFIPath)
	temp, _ := utils.TempDir(b.cfg.Fs, "", "auroraboot-iso")
	err = utils.MkdirAll(b.cfg.Fs, filepath.Join(temp, constants.EfiBootPath), constants.DirPerm)
	if err != nil {
		b.cfg.Logger.Errorf("Failed creating temp efi dir: %v", err)
		return err
	}
	err = utils.MkdirAll(b.cfg.Fs, filepath.Join(isoDir, constants.EfiBootPath), constants.DirPerm)
	if err != nil {
		b.cfg.Logger.Errorf("Failed creating iso efi dir: %v", err)
		return err
	}

	err = b.copyShim(temp, rootdir)
	if err != nil {
		return err
	}

	err = b.copyGrub(temp, rootdir)
	if err != nil {
		return err
	}

	// Generate grub cfg that chainloads into the default livecd grub under /boot/grub2/grub.cfg
	// Its read from the root of the livecd, so we need to copy it into /EFI/BOOT/grub.cfg
	// This is due to the hybrid bios/efi boot mode of the livecd
	// the uefi.img is loaded into memory and run, but grub only sees the livecd root
	err = b.cfg.Fs.WriteFile(filepath.Join(isoDir, constants.EfiBootPath, constants.GrubCfg), []byte(constants.GrubEfiCfg), constants.FilePerm)
	if err != nil {
		b.cfg.Logger.Errorf("Failed writing grub.cfg: %v", err)
		return err
	}
	// Ubuntu efi searches for the grub.cfg file under /EFI/ubuntu/grub.cfg while we store it under /boot/grub2/grub.cfg
	// workaround this by copying it there as well
	// read the kairos-release from the rootfs to know if we are creating a ubuntu based iso
	var flavor string
	flavor, err = sdkutils.OSRelease("FLAVOR", filepath.Join(rootdir, "etc/kairos-release"))
	if err != nil {
		// fallback to os-release
		flavor, err = sdkutils.OSRelease("FLAVOR", filepath.Join(rootdir, "etc/os-release"))
		if err != nil {
			b.cfg.Logger.Warnf("Failed reading os-release from %s and %s: %v", filepath.Join(rootdir, "etc/kairos-release"), filepath.Join(rootdir, "etc/os-release"), err)
			return err
		}
	}
	b.cfg.Logger.Infof("Detected Flavor: %s", flavor)
	if strings.Contains(strings.ToLower(flavor), "ubuntu") {
		b.cfg.Logger.Infof("Ubuntu based ISO detected, copying grub.cfg to /EFI/ubuntu/grub.cfg")
		err = utils.MkdirAll(b.cfg.Fs, filepath.Join(isoDir, "EFI/ubuntu/"), constants.DirPerm)
		if err != nil {
			b.cfg.Logger.Errorf("Failed writing grub.cfg: %v", err)
			return err
		}
		err = b.cfg.Fs.WriteFile(filepath.Join(isoDir, "EFI/ubuntu/", constants.GrubCfg), []byte(constants.GrubEfiCfg), constants.FilePerm)
		if err != nil {
			b.cfg.Logger.Errorf("Failed writing grub.cfg: %v", err)
			return err
		}
	}

	// Calculate EFI image size based on artifacts
	efiSize, err := utils.DirSize(b.cfg.Fs, temp)
	if err != nil {
		return err
	}
	// align efiSize to the next 4MB slot
	align := int64(4 * 1024 * 1024)
	efiSizeMB := (efiSize/align*align + align) / (1024 * 1024)
	// Create the actual efi image
	err = b.e.CreateFileSystemImage(&v1types.Image{
		File:  img,
		Size:  uint(efiSizeMB),
		FS:    constants.EfiFs,
		Label: constants.EfiLabel,
	})
	if err != nil {
		return err
	}
	b.cfg.Logger.Debugf("EFI image created at %s", img)
	// copy the files from the temporal efi dir into the EFI image
	files, err := b.cfg.Fs.ReadDir(temp)
	if err != nil {
		return err
	}

	for _, f := range files {
		// This copies the efi files into the efi img used for the boot
		b.cfg.Logger.Debugf("Copying %s to %s", filepath.Join(temp, f.Name()), img)
		_, err = b.cfg.Runner.Run("mcopy", "-s", "-i", img, filepath.Join(temp, f.Name()), "::")
		if err != nil {
			b.cfg.Logger.Errorf("Failed copying %s to %s: %v", filepath.Join(temp, f.Name()), img, err)
			return err
		}
	}

	return nil
}

// copyShim copies the shim files into the EFI partition
// tempdir is the temp dir where the EFI image is generated from
// rootdir is the rootfs where the shim files are searched for
func (b BuildISOAction) copyShim(tempdir, rootdir string) error {
	var fallBackShim string
	var err error
	// Get possible shim file paths
	shimFiles := sdkutils.GetEfiShimFiles(b.cfg.Arch)
	// Calculate shim path based on arch
	var shimDest string
	switch b.cfg.Arch {
	case constants.ArchAmd64, constants.Archx86:
		shimDest = filepath.Join(tempdir, constants.ShimEfiDest)
		fallBackShim = filepath.Join("/efi", constants.EfiBootPath, "bootx64.efi")
	case constants.ArchArm64:
		shimDest = filepath.Join(tempdir, constants.ShimEfiArmDest)
		fallBackShim = filepath.Join("/efi", constants.EfiBootPath, "bootaa64.efi")
	default:
		err = fmt.Errorf("not supported architecture: %v", b.cfg.Arch)
	}
	var shimDone bool
	for _, f := range shimFiles {
		_, err := b.cfg.Fs.Stat(filepath.Join(rootdir, f))
		if err != nil {
			b.cfg.Logger.Debugf("skip copying %s: not found", filepath.Join(rootdir, f))
			continue
		}
		b.cfg.Logger.Debugf("Copying %s to %s", filepath.Join(rootdir, f), shimDest)
		err = utils.CopyFile(
			b.cfg.Fs,
			filepath.Join(rootdir, f),
			shimDest,
		)
		if err != nil {
			b.cfg.Logger.Warnf("error reading %s: %s", filepath.Join(rootdir, f), err)
			continue
		}
		shimDone = true
		break
	}
	if !shimDone {
		// All failed...maybe we are on alpine which doesnt provide shim/grub.efi ?
		// In that case, we can just use the luet packaged artifacts
		err = utils.CopyFile(
			b.cfg.Fs,
			fallBackShim,
			shimDest,
		)
		if err != nil {
			b.cfg.Logger.Debugf("List of shim files searched for in %s: %s", rootdir, shimFiles)
			return fmt.Errorf("could not find any shim file to copy")
		}
		b.cfg.Logger.Debugf("Using fallback shim file %s", fallBackShim)
		// Also copy the shim.efi file into the rootfs so the installer can find it. Side effect of
		// alpine not providing shim/grub.efi and we not providing it from packages anymore
		_ = utils.MkdirAll(b.cfg.Fs, filepath.Join(rootdir, filepath.Dir(shimFiles[0])), constants.DirPerm)
		err = utils.CopyFile(
			b.cfg.Fs,
			fallBackShim,
			filepath.Join(rootdir, shimFiles[0]),
		)
		if err != nil {
			b.cfg.Logger.Debugf("Could not copy fallback shim into rootfs from %s to %s", fallBackShim, filepath.Join(rootdir, shimFiles[0]))
			return fmt.Errorf("could not copy fallback shim into rootfs from %s to %s", fallBackShim, filepath.Join(rootdir, shimFiles[0]))
		}
	}
	return err
}

// copyGrub copies the shim files into the EFI partition
// tempdir is the temp dir where the EFI image is generated from
// rootdir is the rootfs where the shim files are searched for
func (b BuildISOAction) copyGrub(tempdir, rootdir string) error {
	// this is shipped usually with osbuilder and the files come from livecd/grub2-efi-artifacts
	var fallBackGrub = filepath.Join("/efi", constants.EfiBootPath, "grub.efi")
	var err error
	// Get possible grub file paths
	grubFiles := sdkutils.GetEfiGrubFiles(b.cfg.Arch)
	var grubDone bool
	for _, f := range grubFiles {
		stat, err := b.cfg.Fs.Stat(filepath.Join(rootdir, f))
		if err != nil {
			b.cfg.Logger.Debugf("skip copying %s: not found", filepath.Join(rootdir, f))
			continue
		}
		// Same name as the source, shim looks for that name. We need to remove the .signed suffix
		nameDest := filepath.Join(tempdir, "EFI/BOOT", cleanupGrubName(stat.Name()))
		b.cfg.Logger.Debugf("Copying %s to %s", filepath.Join(rootdir, f), nameDest)

		err = utils.CopyFile(
			b.cfg.Fs,
			filepath.Join(rootdir, f),
			nameDest,
		)
		if err != nil {
			b.cfg.Logger.Warnf("error reading %s: %s", filepath.Join(rootdir, f), err)
			continue
		}
		grubDone = true
		break
	}
	if !grubDone {
		// All failed...maybe we are on alpine which doesnt provide shim/grub.efi ?
		// In that case, we can just use the luet packaged artifacts
		err = utils.CopyFile(
			b.cfg.Fs,
			fallBackGrub,
			filepath.Join(tempdir, "EFI/BOOT/grub.efi"),
		)
		if err != nil {
			b.cfg.Logger.Debugf("List of grub files searched for: %s", grubFiles)
			return fmt.Errorf("could not find any grub efi file to copy")
		}
		b.cfg.Logger.Debugf("Using fallback grub file %s", fallBackGrub)
		// Also copy the grub.efi file into the rootfs so the installer can find it. Side effect of
		// alpine not providing shim/grub.efi and we not providing it from packages anymore
		utils.MkdirAll(b.cfg.Fs, filepath.Join(rootdir, filepath.Dir(grubFiles[0])), constants.DirPerm)
		err = utils.CopyFile(
			b.cfg.Fs,
			fallBackGrub,
			filepath.Join(rootdir, grubFiles[0]),
		)
		if err != nil {
			b.cfg.Logger.Debugf("Could not copy fallback grub into rootfs from %s to %s", fallBackGrub, filepath.Join(rootdir, grubFiles[0]))
			return fmt.Errorf("could not copy fallback shim into rootfs from %s to %s", fallBackGrub, filepath.Join(rootdir, grubFiles[0]))
		}
	}
	return err
}

func (b BuildISOAction) burnISO(root string) error {
	cmd := "xorriso"
	var outputFile string
	var isoFileName string

	if b.cfg.Date {
		currTime := time.Now()
		isoFileName = fmt.Sprintf("%s.%s.iso", b.cfg.Name, currTime.Format("20060102"))
	} else {
		isoFileName = fmt.Sprintf("%s.iso", b.cfg.Name)
	}

	outputFile = isoFileName
	if b.cfg.OutDir != "" {
		outputFile = filepath.Join(b.cfg.OutDir, outputFile)
	}

	if exists, _ := utils.Exists(b.cfg.Fs, outputFile); exists {
		b.cfg.Logger.Warnf("Overwriting already existing %s", outputFile)
		err := b.cfg.Fs.Remove(outputFile)
		if err != nil {
			return err
		}
	}

	args := []string{
		"-volid", b.spec.Label, "-joliet", "on", "-padding", "0",
		"-outdev", outputFile, "-map", root, "/", "-chmod", "0755", "--",
	}
	args = append(args, constants.GetXorrisoBooloaderArgs(root)...)

	out, err := b.cfg.Runner.Run(cmd, args...)
	b.cfg.Logger.Debugf("Xorriso: %s", string(out))
	if err != nil {
		return err
	}

	checksum, err := utils.CalcFileChecksum(b.cfg.Fs, outputFile)
	if err != nil {
		return fmt.Errorf("checksum computation failed: %w", err)
	}
	err = b.cfg.Fs.WriteFile(fmt.Sprintf("%s.sha256", outputFile), []byte(fmt.Sprintf("%s %s\n", checksum, isoFileName)), 0644)
	if err != nil {
		return fmt.Errorf("cannot write checksum file: %w", err)
	}

	return nil
}

func (b BuildISOAction) applySources(target string, sources ...*v1types.ImageSource) error {
	for _, src := range sources {
		_, err := b.e.DumpSource(target, src)
		if err != nil {
			return err
		}
	}
	return nil
}

// cleanupGrubName will cleanup the grub name to provide a proper grub named file
// As the original name can contain several suffixes to indicate its signed status
// we need to clean them up before using them as the shim will look for a file with
// no suffixes
func cleanupGrubName(name string) string {
	// remove the .signed suffix if present
	clean := strings.TrimSuffix(name, ".signed")
	// remove the .dualsigned suffix if present
	clean = strings.TrimSuffix(clean, ".dualsigned")
	// remove the .signed.latest suffix if present
	clean = strings.TrimSuffix(clean, ".signed.latest")
	return clean
}

func WithLogger(logger sdkTypes.KairosLogger) func(r *agentconfig.Config) error {
	return func(r *agentconfig.Config) error {
		r.Logger = logger
		return nil
	}
}
