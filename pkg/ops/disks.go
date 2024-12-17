package ops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/kairos-sdk/utils"
)

func PrepareArmPartitions(src, dstPath string, do schema.Config) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		env := genPrepareImageEnv(src, *do.Disk.ARM)
		os.Mkdir("bootloader", 0650)
		internal.Log.Logger.Info().Msgf("Preparing ARM raw disks from '%s' to '%s'", src, dstPath)
		out, err := utils.SH(fmt.Sprintf("%s /prepare_arm_images.sh", strings.Join(env, " ")))
		internal.Log.Logger.Printf("Output '%s'", out)
		if err != nil {
			internal.Log.Logger.Error().Msgf("Preparing raw disks from '%s' to '%s' failed: %s", src, dstPath, err.Error())
		}

		out, err = utils.SH(fmt.Sprintf("mv bootloader/*.img %s", dstPath))
		internal.Log.Logger.Printf("Output '%s'", out)
		if err != nil {
			internal.Log.Logger.Error().Msgf("Preparing raw disks from '%s' to '%s' failed: %s", src, dstPath, err.Error())
		}
		return err
	}
}

func genPrepareImageEnv(src string, do schema.ARMDiskOptions) []string {
	args := []string{fmt.Sprintf("directory=%s", src)}

	if do.DiskSize.Disk != "" {
		args = append(args, fmt.Sprintf("size=%s", do.DiskSize.Disk))
	}

	if do.DiskSize.StatePartition != "" {
		args = append(args, fmt.Sprintf("state_size=%s", do.DiskSize.StatePartition))
	}

	if do.DiskSize.RecoveryPartition != "" {
		args = append(args, fmt.Sprintf("recovery_size=%s", do.DiskSize.RecoveryPartition))
	}

	if do.DiskSize.Images != "" {
		args = append(args, fmt.Sprintf("default_active_size=%s", do.DiskSize.Images))
	}

	return args
}

func genARMBuildArgs(src, cloudConfig string, do schema.ARMDiskOptions) []string {
	args := []string{fmt.Sprintf("--directory %s", src)}

	if do.DiskSize.Disk != "" {
		args = append(args, fmt.Sprintf("--size %s", do.DiskSize.Disk))
	}

	if do.DiskSize.StatePartition != "" {
		args = append(args, fmt.Sprintf("--state-partition-size %s", do.DiskSize.StatePartition))
	}

	if do.DiskSize.RecoveryPartition != "" {
		args = append(args, fmt.Sprintf("--recovery-partition-size %s", do.DiskSize.RecoveryPartition))
	}

	if do.DiskSize.Images != "" {
		args = append(args, fmt.Sprintf("--images-size %s", do.DiskSize.Images))
	}

	if do.Model != "" {
		args = append(args, fmt.Sprintf("--model %s", do.Model))
	}
	if do.LVM {
		args = append(args, "--use-lvm")
	}

	if do.EFIOverlay != "" {
		args = append(args, fmt.Sprintf("--efi-dir %s", do.EFIOverlay))
	}

	args = append(args, fmt.Sprintf("--config %s", cloudConfig))

	return args

}

func GenArmDisk(src, dst string, do schema.Config) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		args := genARMBuildArgs(src, filepath.Join(filepath.Dir(dst), "config.yaml"), *do.Disk.ARM)

		internal.Log.Logger.Info().Msgf("Generating ARM disk '%s' from '%s'", dst, src)
		internal.Log.Logger.Printf("Running 'build-arm-image.sh %s %s'", strings.Join(args, " "), dst)
		out, err := utils.SH(fmt.Sprintf("/build-arm-image.sh %s %s", strings.Join(args, " "), dst))
		internal.Log.Logger.Printf("Output '%s'", out)
		if err != nil {
			internal.Log.Logger.Error().Msgf("Generating ARM disk '%s' from '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

func GenBIOSRawDisk(config schema.Config, srcISO, dst string) func(ctx context.Context) error {
	cloudConfigFile := filepath.Join(filepath.Dir(dst), "config.yaml")
	return func(ctx context.Context) error {

		ram := "8096"
		if config.System.Memory != "" {
			ram = config.System.Memory
		}
		cores := "3"
		if config.System.Cores != "" {
			cores = config.System.Cores
		}

		qemuBin := "qemu-system-x86_64"
		if config.System.Qemubin != "" {
			qemuBin = config.System.Qemubin
		}

		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		internal.Log.Logger.Info().Msgf("Generating MBR disk '%s' from '%s'", dst, srcISO)

		extra := ""
		if config.System.KVM {
			extra = "-enable-kvm"
		}
		out, err := utils.SH(
			fmt.Sprintf(`mkdir -p build
pushd build
touch meta-data
cp -rfv %s user-data

mkisofs -output ci.iso -volid cidata -joliet -rock user-data meta-data
truncate -s "+$((20000*1024*1024))" %s

%s -m %s -smp cores=%s \
		-chardev stdio,mux=on,id=char0,logfile=/tmp/serial.log,signal=off -serial chardev:char0	-mon chardev=char0 \
        -nographic \
        -rtc base=utc,clock=rt \
        -chardev socket,path=qga.sock,server,nowait,id=qga0 \
        -device virtio-serial \
        -device virtserialport,chardev=qga0,name=org.qemu.guest_agent.0 \
        -drive if=virtio,media=disk,file=%s \
        -drive format=raw,media=cdrom,readonly=on,file=%s \
        -drive format=raw,media=cdrom,readonly=on,file=ci.iso \
        -boot d %s
        
`, cloudConfigFile, dst, qemuBin, ram, cores, dst, srcISO, extra),
		)
		internal.Log.Logger.Printf("Output '%s'", out)
		if err != nil {
			internal.Log.Logger.Error().Msgf("Generating raw disk '%s' from '%s' to '%s' failed with error '%s'", dst, srcISO, extra, err.Error())
		}
		return err
	}
}

func GenEFIRawDisk(src, dst string, size uint64) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		internal.Log.Logger.Info().Msgf("Generating raw disk '%s' from '%s' with final size %dMb", dst, src, size)
		raw := NewEFIRawImage(src, dst, size)
		err := raw.Build()
		if err != nil {
			internal.Log.Logger.Error().Msgf("Generating raw disk '%s' from '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

func ExtractSquashFS(src, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		internal.Log.Logger.Info().Msgf("unpacking to '%s' the squashfs file: '%s'", dst, src)
		out, err := utils.SH(fmt.Sprintf("unsquashfs -f -d %s %s", dst, src))
		internal.Log.Logger.Printf("Output '%s'", out)
		if err != nil {
			internal.Log.Logger.Error().Msgf("unpacking to '%s' from '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

func ConvertRawDiskToVHD(src, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		internal.Log.Logger.Info().Msgf("Generating raw disk '%s' from '%s'", dst, src)
		out, err := utils.SH(fmt.Sprintf("/azure.sh %s %s", src, dst))
		internal.Log.Logger.Printf("Output '%s'", out)
		if err != nil {
			internal.Log.Logger.Error().Msgf("Generating raw disk '%s' from '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}

func ConvertRawDiskToGCE(src, dst string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		tmp, err := os.MkdirTemp("", "gendisk")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)

		internal.Log.Logger.Info().Msgf("Generating raw disk '%s' from '%s'", dst, src)
		out, err := utils.SH(fmt.Sprintf("/gce.sh %s %s", src, dst))
		internal.Log.Logger.Printf("Output '%s'", out)
		if err != nil {
			internal.Log.Logger.Error().Msgf("Generating raw disk '%s' from '%s' failed with error '%s'", dst, src, err.Error())
		}
		return err
	}
}
