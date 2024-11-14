#!/bin/bash

image=$1

if [ -z "$image" ]; then
    echo "No image specified"
    exit 1
fi

LOADER_OFFSET=${LOADER_OFFSET:-"64"}
LOADER_IMAGE=${LOADER_IMAGE:-"idbloader.img"}
UBOOT_IMAGE=${UBOOT_IMAGE:-"u-boot.itb"}
UBOOT_OFFSET=${UBOOT_OFFSET:-"16384"}

echo "Writing idbloader"
dd conv=notrunc if=/pinebookpro/u-boot/usr/lib/u-boot/pinebook-pro-rk3399/${LOADER_IMAGE} of="$image" conv=fsync seek=${LOADER_OFFSET}
echo "Writing u-boot image"
dd conv=notrunc if=/pinebookpro/u-boot/usr/lib/u-boot/pinebook-pro-rk3399/${UBOOT_IMAGE} of="$image" conv=fsync seek=${UBOOT_OFFSET}
sync $image