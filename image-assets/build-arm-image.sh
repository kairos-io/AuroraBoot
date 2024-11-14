#!/bin/bash

## This is a re-adaptation of https://github.com/rancher/elemental-toolkit/blob/main/images/arm-img-builder.sh

set -ex

load_vars() {

  model=${MODEL:-odroid_c2}
  directory=${DIRECTORY:-}
  output_image="${OUTPUT_IMAGE:-arm.img}"
  # Img creation options. Size is in MB for all of the vars below
  size="${SIZE:-7608}"
  state_size="${STATE_SIZE:-4992}"
  oem_size="${OEM_SIZE:-64}"
  recovery_size="${RECOVERY_SIZE:-2192}"
  default_active_size="${DEFAULT_ACTIVE_SIZE:-2400}"
  menu_entry="${DEFAULT_MENU_ENTRY:-Kairos}"

  ## Repositories
  final_repo="${FINAL_REPO:-quay.io/costoolkit/releases-teal-arm64}"
  repo_type="${REPO_TYPE:-docker}"

  # Warning: these default values must be aligned with the values provided
  # in 'packages/cos-config/cos-config', provide an environment file using the
  # --cos-config flag if different values are needed.
  : "${OEM_LABEL:=COS_OEM}"
  : "${RECOVERY_LABEL:=COS_RECOVERY}"
  : "${ACTIVE_LABEL:=COS_ACTIVE}"
  : "${PASSIVE_LABEL:=COS_PASSIVE}"
  : "${PERSISTENT_LABEL:=COS_PERSISTENT}"
  : "${SYSTEM_LABEL:=COS_SYSTEM}"
  : "${STATE_LABEL:=COS_STATE}"
}

cleanup() {
	sync
	sync
	sleep 5
	sync
  if [ -n "$EFI" ]; then
    rm -rf $EFI
  fi
  if [ -n "$RECOVERY" ]; then
    rm -rf $RECOVERY
  fi
  if [ -n "$STATEDIR" ]; then
    rm -rf $STATEDIR
  fi
  if [ -n "$TARGET" ]; then
    umount $TARGET || true
    umount $LOOP || true
    rm -rf $TARGET
  fi
  if [ -n "$WORKDIR" ]; then
    rm -rf $WORKDIR
  fi
  if [ -n "$DRIVE" ]; then
    umount $DRIVE || true
  fi
  if [ -n "$recovery" ]; then
    umount $recovery || true
  fi
  if [ -n "$state" ]; then
    umount $state || true
  fi
  if [ -n "$efi" ]; then
    umount $efi || true
  fi
  if [ -n "$oem" ]; then
    umount $oem || true
  fi

  losetup -D "${LOOP}" || true;
  losetup -D "${DRIVE}" || true;

  if [ "$model" == "rpi3" ]; then
    dmsetup remove KairosVG-oem || true;
    dmsetup remove KairosVG-recovery || true;
  fi
}

ensure_dir_structure() {
    local target=$1
    for mnt in /sys /proc /dev /tmp /boot /usr/local /oem
    do
        if [ ! -d "${target}${mnt}" ]; then
          mkdir -p ${target}${mnt}
        fi
    done
}

usage()
{
    echo "Usage: $0 [options] image.img"
    echo ""
    echo "Example: $0 --cos-config cos-config  --model odroid-c2 --docker-image <image> output.img"
    echo ""
    echo "Flags:"
    echo " --cos-config: (optional) Specifies a cos-config file for required environment variables"
    echo " --config: (optional) Specify a cloud-init config file to embed into the final image"
    echo " --manifest: (optional) Specify a manifest file to customize efi/grub packages installed into the image"
    echo " --size: (optional) Image size (MB)"
    echo " --state-partition-size: (optional) Size of the state partition (MB)"
    echo " --recovery-partition-size: (optional) Size of the recovery partition (MB)"
    echo " --images-size: (optional) Size of the active/passive/recovery images (MB)"
    echo " --docker-image: (optional) A container image which will be used for active/passive/recovery system"
    echo " --directory: (optional) A directory which will be used for active/passive/recovery system"
    echo " --model: (optional) The board model"
    echo " --efi-dir: (optional) A directory with files which will be added to the efi partition"
    exit 1
}

get_url()
{
    FROM=$1
    TO=$2
    case $FROM in
        ftp*|http*|tftp*)
            n=0
            attempts=5
            until [ "$n" -ge "$attempts" ]
            do
                curl -o $TO -fL ${FROM} && break
                n=$((n+1))
                echo "Failed to download, retry attempt ${n} out of ${attempts}"
                sleep 2
            done
            ;;
        *)
            cp -f $FROM $TO
            ;;
    esac
}

trap "cleanup" 1 2 3 6 14 15 EXIT

load_vars

while [ "$#" -gt 0 ]; do
    case $1 in
        --cos-config)
            shift 1
            cos_config=$1
            ;;
        --config)
            shift 1
            config=$1
            ;;
        --manifest)
            shift 1
            manifest=$1
            ;;
        --size)
            shift 1
            size=$1
            ;;
        --state-partition-size)
            shift 1
            state_size=$1
            ;;
        --recovery-partition-size)
            shift 1
            recovery_size=$1
            ;;
        --images-size)
            shift 1
            default_active_size=$1
            ;;
        --docker-image)
            shift 1
            CONTAINER_IMAGE=$1
            ;;
        --directory)
            shift 1
            directory=$1
            ;;
        --model)
            shift 1
            model=$1
            ;;
        --efi-dir)
            shift 1
            efi_dir=$1
            ;;
        --final-repo)
            shift 1
            final_repo=$1
            ;;
        --repo-type)
            shift 1
            repo_type=$1
            ;;
        -h)
            usage
            ;;
        --help)
            usage
            ;;
        *)
            if [ "$#" -gt 2 ]; then
                usage
            fi
            output_image=$1
            break
            ;;
    esac
    shift 1
done

if [ "$model" == "rpi64" ];then
  echo "rpi64 model not supported anymore, please select either rpi3 or rpi4"
  exit 1
fi

if [ "$model" == "rpi3" ] || [ "$model" == "rpi4" ]; then
    container_image=${CONTAINER_IMAGE:-quay.io/costoolkit/examples:rpi-latest}
else
    # Odroid C2 image contains kernel-default-extra, might have broader support
    container_image=${CONTAINER_IMAGE:-quay.io/costoolkit/examples:odroid-c2-latest}
fi

if [ -n "$cos_config" ] && [ -e "$cos_config" ]; then
  # shellcheck source=/dev/null
  source "$cos_config"
fi

if [ -z "$output_image" ]; then
  echo "No image file specified"
  exit 1
fi

if [ -n "$manifest" ]; then
  YQ_PACKAGES_COMMAND=(yq e -o=json "$manifest")
  final_repo=${final_repo:-$(yq e ".raw_disk.$model.repo" "${manifest}")}
fi

echo "@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@"
echo "Image Size: $size MB."
echo "Recovery Partition: $recovery_size."
echo "State Partition: $state_size MB."
echo "Images size (active/passive/recovery.img): $default_active_size MB."
echo "Model: $model"
if [ -n "$container_image" ] && [ -z "$directory" ]; then
  echo "Container image: $container_image"
fi
if [ -n "$directory" ]; then
  echo "Root from directory: $directory"
fi
echo "@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@"

# Temp dir used during build
WORKDIR=$(mktemp -d --tmpdir arm-builder.XXXXXXXXXX)
#ROOT_DIR=$(git rev-parse --show-toplevel)
TARGET=$(mktemp -d --tmpdir arm-builder.XXXXXXXXXX)
STATEDIR=$(mktemp -d --tmpdir arm-builder.XXXXXXXXXX)


export WORKDIR

# Prepare active.img

echo ">> Preparing active.img"
mkdir -p ${STATEDIR}/cOS

dd if=/dev/zero of=${STATEDIR}/cOS/active.img bs=1M count=$default_active_size

mkfs.ext2 ${STATEDIR}/cOS/active.img -L ${ACTIVE_LABEL}

sync

LOOP=$(losetup --show -f ${STATEDIR}/cOS/active.img)
if [ -z "$LOOP" ]; then
echo "No device"
exit 1
fi

mount -t ext2 $LOOP $TARGET

ensure_dir_structure $TARGET

# Download the container image
if [ -z "$directory" ]; then
  echo ">>> Downloading container image"
  kairos-agent pull-image $container_image $TARGET
else
  echo ">>> Copying files from $directory"
  rsync -axq --exclude='host' --exclude='mnt' --exclude='proc' --exclude='sys' --exclude='dev' --exclude='tmp' ${directory}/ $TARGET
fi

# We copy the grubmenu.cfg to a temporary location to be copied later in the state partition
# https://github.com/kairos-io/kairos/blob/62c67e3e61d49435c362014522e5c6696335376f/overlay/files/system/oem/08_grub.yaml#L105
# This is a hack and we need a better way: https://github.com/kairos-io/kairos/issues/1427
tmpgrubconfig=$(mktemp /tmp/grubmeny.cfg.XXXXXX)
cp -rfv $TARGET/etc/kairos/branding/grubmenu.cfg "${tmpgrubconfig}"

umount $TARGET
sync

if [ -n "$LOOP" ]; then
    losetup -d $LOOP
fi

echo ">> Preparing passive.img"
cp -rfv ${STATEDIR}/cOS/active.img ${STATEDIR}/cOS/passive.img
tune2fs -L ${PASSIVE_LABEL} ${STATEDIR}/cOS/passive.img

# Preparing recovery
echo ">> Preparing recovery.img"
RECOVERY=$(mktemp -d --tmpdir arm-builder.XXXXXXXXXX)
if [ -z "$RECOVERY" ]; then
  echo "No recovery directory"
  exit 1
fi

mkdir -p ${RECOVERY}/cOS
cp -rfv ${STATEDIR}/cOS/active.img ${RECOVERY}/cOS/recovery.img

tune2fs -L ${SYSTEM_LABEL} ${RECOVERY}/cOS/recovery.img

# Install real grub config to recovery
cp -rfv /arm/raw/grubconfig/* $RECOVERY
mkdir -p $RECOVERY/grub2/fonts
cp -rfv /arm/raw/grubartifacts/* $RECOVERY/grub2
mv $RECOVERY/grub2/*pf2 $RECOVERY/grub2/fonts

sync

# Prepare efi files
echo ">> Preparing EFI partition"
EFI=$(mktemp -d --tmpdir arm-builder.XXXXXXXXXX)
if [ -z "$EFI" ]; then
  echo "No EFI directory"
  exit 1
fi

cp -rfv /arm/raw/grubefi/* $EFI
if [ -n "$EFI" ] && [  -n "$efi_dir" ]; then
  echo "Copy $efi_dir to EFI directory"
  cp -rfv $efi_dir/* $EFI
fi

partprobe

echo ">> Writing image and partition table"
dd if=/dev/zero of="${output_image}" bs=1024000 count="${size}" || exit 1

# Image partitions
# only rpi4 supports gpt
if [ "$model" == "rpi3" ]; then
    sgdisk -n 1:8192:+96M -c 1:EFI -t 1:0c00 ${output_image}
    sgdisk -n 2:0:+${state_size}M -c 2:state -t 2:8300 ${output_image}
    sgdisk -n 3:0:+$(( recovery_size + oem_size ))M -c 3:lvm -t 3:8e00 ${output_image}
    sgdisk -n 4:0:+64M -c 4:persistent -t 4:8300 ${output_image}
    sgdisk -m 1:2:3:4 ${output_image}
    sfdisk --part-type ${output_image} 1 c
elif [ "$model" == "rpi4" ]; then
    sgdisk -n 1:0:+96M -c 1:EFI -t 1:ef00 ${output_image}
    partprobe
    sgdisk -n 2:0:+${state_size}M -c 2:state -t 2:8300 ${output_image}
    partprobe
    sgdisk -n 3:0:+${recovery_size}M -c 3:recovery -t 3:8300 ${output_image}
    partprobe
    sgdisk -n 4:0:+${oem_size}M -c 4:oem -t 4:8300 ${output_image}
    partprobe
    sgdisk -n 5:0:+64M -c 5:persistent -t 5:8300 ${output_image}
    partprobe
    # Move backup header to end of disk
    sgdisk -e ${output_image}
    sgdisk -v ${output_image}
else
    sgdisk -n 1:8192:+16M -c 1:EFI -t 1:0700 ${output_image}
    sgdisk -n 2:0:+${state_size}M -c 2:state -t 2:8300 ${output_image}
    sgdisk -n 3:0:+$(( recovery_size + oem_size ))M -c 3:lvm -t 3:8e00 ${output_image}
    sgdisk -n 4:0:+64M -c 4:persistent -t 4:8300 ${output_image}
    sgdisk -m 1:2:3:4 ${output_image}
fi

# Prepare the image and copy over the files

DRIVE=$(losetup -f "${output_image}" --show)
export DRIVE
if [ -z "${DRIVE}" ]; then
	echo "Cannot execute losetup for $output_image"
	exit 1
fi

device=${DRIVE/\/dev\//}

if [ -z "$device" ]; then
  echo "No device"
  exit 1
fi

export device="/dev/mapper/${device}"

partprobe

if [ "$model" == 'rpi4' ]; then
  kpartx -vag $DRIVE
else
  kpartx -va $DRIVE
fi

echo ">> Populating partitions"
efi=${device}p1
state=${device}p2
recovery=${device}p3

if [ "$model" == 'rpi4' ]; then
  oem=${device}p4
  persistent=${device}p5
else
  persistent=${device}p4
  oem_lv=/dev/mapper/KairosVG-oem
  recovery_lv=/dev/mapper/KairosVG-recovery
fi

# Create partitions (RECOVERY, STATE, COS_PERSISTENT)
mkfs.vfat -F 32 ${efi}
fatlabel ${efi} COS_GRUB
mkfs.ext3 -F -L ${STATE_LABEL} $state
mkfs.ext3 -F -L ${PERSISTENT_LABEL} $persistent

if [ "$model" == 'rpi4' ]; then
  mkfs.ext3 -F -L ${RECOVERY_LABEL} $recovery
  mkfs.ext3 -F -L ${OEM_LABEL} $oem
else
  pvcreate $recovery
  vgcreate KairosVG $recovery
  lvcreate -Z n -n oem -L ${oem_size} KairosVG
  lvcreate -Z n -n recovery -l 100%FREE KairosVG
  vgchange -ay
  vgmknodes
  mkfs.ext4 -F -L ${OEM_LABEL} $oem_lv
  mkfs.ext4 -F -L ${RECOVERY_LABEL} $recovery_lv
fi

mkdir $WORKDIR/state
mkdir $WORKDIR/recovery
mkdir $WORKDIR/efi
mkdir $WORKDIR/oem

mount $state $WORKDIR/state
mount $efi $WORKDIR/efi

if [ "$model" == 'rpi4' ]; then
  mount $recovery $WORKDIR/recovery
  mount $oem $WORKDIR/oem
else
  mount $recovery_lv $WORKDIR/recovery
  mount $oem_lv $WORKDIR/oem
fi

cp -rfv /defaults.yaml $WORKDIR/oem/01_defaults.yaml

# Set a OEM config file if specified
if [ -n "$config" ]; then
  echo ">> Copying $config OEM config file"
  get_url $config $WORKDIR/oem/99_custom.yaml
fi

grub2-editenv $WORKDIR/state/grub_oem_env set "default_menu_entry=$menu_entry"

# We copy the file we saved earier to the STATE partition
cp -rfv "${tmpgrubconfig}" $WORKDIR/state/grubmenu

# Copy over content
cp -arf $EFI/* $WORKDIR/efi
cp -arf $RECOVERY/* $WORKDIR/recovery
cp -arf $STATEDIR/* $WORKDIR/state

umount $WORKDIR/recovery
umount $WORKDIR/state
umount $WORKDIR/efi
umount $WORKDIR/oem


if [ "$model" != 'rpi4' ]; then
  vgchange -an
fi

sync

# Flash uboot and vendor-specific bits
echo ">> Performing $model specific bits.."
/arm/boards/$model.sh ${DRIVE}

sync
sleep 5
sync

if [ "$model" == 'rpi4' ]; then
  kpartx -dvg $DRIVE
else
  kpartx -dv $DRIVE || true
fi

umount $DRIVE || true

echo ">> Done writing $output_image"

echo ">> Creating SHA256 sum"

sha256sum $output_image > $output_image.sha256

cleanup
