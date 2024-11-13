#!/bin/bash

partprobe

kpartx -va $DRIVE

image=$1

if [ -z "$image" ]; then
    echo "No image specified"
    exit 1
fi

set -ax
TEMPDIR="$(mktemp -d)"
echo $TEMPDIR
mount "${device}p1" "${TEMPDIR}"

# Copy all rpi files
cp -rfv /rpi/* $TEMPDIR

umount "${TEMPDIR}"
