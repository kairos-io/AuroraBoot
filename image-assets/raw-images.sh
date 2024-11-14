#!/bin/bash
# Generates EFI bootable images (statically)
# This is a re-adaptation of https://github.com/rancher/elemental-toolkit/blob/v0.8.10-1/images/img-builder.sh, which was dropped
# How to use:
# First extract the image which you want to create an image from:
### luet util unpack <image> rootfs
# Then convert it to a raw disk (EFI only):
### docker run -v $PWD:/output --entrypoint /raw-images.sh -ti --rm test-image /output/rootfs /output/foo.raw cloud-init.yaml

: "${OEM_LABEL:=COS_OEM}"
: "${RECOVERY_LABEL:=COS_RECOVERY}"
: "${EXTEND:=}"
: "${RECOVERY_SIZE:=2048}"

DIRECTORY=$1
OUT=${2:-disk.raw}
CONFIG=$3

echo "Output: $OUT"

set -e

mkdir -p /build/root/grub2
mkdir /build/root/cOS
mkdir /build/efi

cp -rf /raw/grub/* /build/efi
cp -rf /raw/grubconfig/* /build/root
cp -rf /raw/grubartifacts/* /build/root/grub2

echo "Generating squashfs from $DIRECTORY"
mksquashfs $DIRECTORY recovery.squashfs -b 1024k -comp xz -Xbcj x86
mv recovery.squashfs /build/root/cOS/recovery.squashfs

grub2-editenv /build/root/grub_oem_env set "default_menu_entry=Kairos"

# Create a 2GB filesystem for RECOVERY including the contents for root (grub config and squasfs container)
# shellcheck disable=SC2004
truncate -s $(($RECOVERY_SIZE*1024*1024)) rootfs.part
mkfs.ext2 -L "${RECOVERY_LABEL}" -d /build/root rootfs.part

# Create the EFI partition FAT16 and include the EFI image and a basic grub.cfg
truncate -s $((20*1024*1024)) efi.part

mkfs.fat -F16 -n COS_GRUB efi.part
mcopy -s -i efi.part /build/efi/EFI ::EFI

# Create the grubenv forcing first boot to be on recovery system
mkdir -p /build/oem
cp /build/root/etc/cos/grubenv_firstboot /build/oem/grubenv
if [ -n "$CONFIG" ]; then
  echo "Copying config file ($CONFIG)"
  cp $CONFIG /build/oem
fi

# Create a 64MB filesystem for OEM volume
truncate -s $((64*1024*1024)) oem.part
mkfs.ext2 -L "${OEM_LABEL}" -d /build/oem oem.part

echo "Generating image $OUT"
# Create disk image, add 3MB of initial free space to disk, 1MB is for proper alignement, 2MB are for the hybrid legacy boot.
truncate -s $((3*1024*1024)) $OUT
{
    cat efi.part
    cat oem.part
    cat rootfs.part
} >> $OUT

# Add an extra MB at the end of the disk for the gpt headers, in fact 34 sectors would be enough, but adding some more does not hurt.
truncate -s "+$((1024*1024))" $OUT

if [ -n "$EXTEND" ]; then
  echo "Extending image of $EXTEND MB"
  truncate -s "+$((EXTEND*1024*1024))" $OUT
fi

# Create the partition table in $OUT (assumes sectors of 512 bytes)
sgdisk -n 1:2048:+2M -c 1:legacy -t 1:EF02 $OUT
sgdisk -n 2:0:+20M -c 2:UEFI -t 2:EF00 $OUT
sgdisk -n 3:0:+64M -c 3:oem -t 3:8300 $OUT
sgdisk -n 4:0:+${RECOVERY_SIZE}M -c 4:root -t 4:8300 $OUT
