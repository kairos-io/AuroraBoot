package constants

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// Eltorito image is basically a grub cdboot.img + grub core image
// So basically you build a grub image with modules embedded and prepend it with the cdboot, which allows it to boot from
// CDRom and run the grub embedded in the image directly from BIOS
// This can be generated from any distro by runing something like:
/*
grub2-mkimage -O i386-pc -o core.img -p /boot/grub2 -d PATH_TO_i386_MODULES ext2 iso9660 linux echo configfile search_label search_fs_file search search_fs_uuid ls normal gzio gettext font gfxterm gfxmenu all_video test true loadenv part_gpt part_msdos biosdisk vga vbe chain boot
cat $(find / -name cdboot.img -print) core.img > eltorito.img

Important things in the grub image creation:
 - -O i386-pc is the architecture we want to build the image for. Bios is i386
 - -p is the prefix dir, this is where grub will start searching for things, including the grub.cfg config, when it boots
 - -d is the current dir where modules and images are. Usually this is automatically set so it can be dropped
 - the list at the end are the modules to bundle for grub. Honestly the list is not too big and it can probably be dropped to like half for the livecd
   as it only uses linux, echo, font, video ones and boot. But it doesnt hurt to have extra modules.
*/
//go:embed eltorito.img
var Eltorito []byte

// BootHybrid is boot_hybrid.img which comes bundled with grub
// Its ASM to boot from the grub image embedded
// You can check its source here: https://github.com/rhboot/grub2/blob/fedora-39/grub-core/boot/i386/pc/cdboot.S
//
//go:embed boot_hybrid.img
var BootHybrid []byte

// GrubLiveBiosCfg is the livecd config for BIOS boot
//
//go:embed grub_live_bios.cfg
var GrubLiveBiosCfg []byte

type UkiOutput string

const (
	IsoEFIPath     = "/boot/uefi.img"
	EfiBootPath    = "/EFI/BOOT"
	EfiLabel       = "COS_GRUB"
	EfiFs          = "vfat"
	IsoRootFile    = "rootfs.squashfs"
	ISOLabel       = "COS_LIVE"
	ShimEfiDest    = EfiBootPath + "/bootx64.efi"
	ShimEfiArmDest = EfiBootPath + "/bootaa64.efi"
	BuildImgName   = "elemental"
	GrubCfg        = "grub.cfg"
	GrubPrefixDir  = "/boot/grub2"
	GrubEfiCfg     = "search --no-floppy --file --set=root " + IsoKernelPath +
		"\nset prefix=($root)" + GrubPrefixDir +
		"\nconfigfile $prefix/" + GrubCfg

	// GrubEfiRecovery Used for RAW images as we chainload the grub config in the recovery partition
	GrubEfiRecovery = "search --no-floppy --label --set=root COS_RECOVERY" +
		"\nset root=($root)" +
		"\nset prefix=($root)/grub2\n" +
		"configfile ($root)/etc/cos/grub.cfg"
	IsoBootCatalog = "/boot/boot.catalog"
	IsoHybridMBR   = "/boot/boot_hybrid.img"
	IsoBootFile    = "/boot/eltorito.img"

	// These paths are arbitrary but coupled to grub.cfg
	IsoKernelPath = "/boot/kernel"
	IsoInitrdPath = "/boot/initrd"

	// Default directory and file fileModes
	DirPerm        = os.ModeDir | os.ModePerm
	FilePerm       = 0666
	NoWriteDirPerm = 0555 | os.ModeDir
	TempDirPerm    = os.ModePerm | os.ModeSticky | os.ModeDir

	ArchArm64   = "arm64"
	Archx86     = "x86_64"
	ArchAmd64   = "amd64"
	Archaarch64 = "aarch64"

	UkiCmdline            = "console=ttyS0 console=tty1 net.ifnames=1 rd.immucore.oemlabel=COS_OEM rd.immucore.oemtimeout=2 rd.immucore.uki selinux=0 panic=5 rd.shell=0 systemd.crash_reboot=yes"
	UkiCmdlineInstall     = "install-mode"
	UkiSystemdBootx86     = "/usr/kairos/systemd-bootx64.efi"
	UkiSystemdBootArm     = "/usr/kairos/systemd-bootaa64.efi"
	UkiSystemdBootStubx86 = "/usr/kairos/linuxx64.efi.stub"
	UkiSystemdBootStubArm = "/usr/kairos/linuxaa64.efi.stub"

	EfiFallbackNamex86 = "BOOTX64.EFI"
	EfiFallbackNameArm = "BOOTAA64.EFI"

	ArtifactBaseName   = "norole"
	DefaultCloudConfig = `#cloud-config
stages:
  initramfs:
    - name: "Set user and password"
      users:
        kairos:
          passwd: "kairos"
          groups:
            - "admin"`
)

const IsoOutput UkiOutput = "iso"
const ContainerOutput UkiOutput = "container"
const DefaultOutput UkiOutput = "uki"

const MB = int64(1024 * 1024)
const GB = 1024 * MB

func OutPutTypes() []string {
	return []string{string(IsoOutput), string(ContainerOutput), string(DefaultOutput)}
}

func GetXorrisoBooloaderArgs(root string) []string {
	args := []string{
		"-boot_image", "grub", fmt.Sprintf("bin_path=%s", IsoBootFile),
		"-boot_image", "grub", fmt.Sprintf("grub2_mbr=%s/%s", root, IsoHybridMBR),
		"-boot_image", "grub", "grub2_boot_info=on",
		"-boot_image", "any", "partition_offset=16",
		"-boot_image", "any", fmt.Sprintf("cat_path=%s", IsoBootCatalog),
		"-boot_image", "any", "cat_hidden=on",
		"-boot_image", "any", "boot_info_table=on",
		"-boot_image", "any", "platform_id=0x00",
		"-boot_image", "any", "emul_type=no_emulation",
		"-boot_image", "any", "load_size=2048",
		"-boot_image", "any", "next",
		"-boot_image", "any", "platform_id=0xef",
		"-boot_image", "any", "emul_type=no_emulation",
		"-append_partition", "2", "0xef", filepath.Join(root, IsoEFIPath),
		"-boot_image", "any", "efi_path=--interval:appended_partition_2:all::",
	}
	return args
}

// GetDefaultSquashfsOptions returns the default options to use when creating a squashfs
func GetDefaultSquashfsOptions() []string {
	return []string{"-b", "1024k"}
}
