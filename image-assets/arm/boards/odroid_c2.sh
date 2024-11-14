#!/bin/bash

image=$1

if [ -z "$image" ]; then
    echo "No image specified"
    exit 1
fi

# conv=notrunc ?
dd if=/firmware/odroid-c2/bl1.bin.hardkernel of=$image conv=fsync bs=1 count=442
dd if=/firmware/odroid-c2/bl1.bin.hardkernel of=$image conv=fsync bs=512 skip=1 seek=1
dd if=/firmware/odroid-c2/u-boot.odroidc2 of=$image conv=fsync bs=512 seek=97