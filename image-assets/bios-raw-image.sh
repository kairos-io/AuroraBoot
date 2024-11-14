#!/bin/bash
# Generates raw bootable images with qemu
set -ex
CLOUD_INIT=${1:-cloud_init.yaml}
QEMU=${QEMU:-qemu-system-x86_64}
ISO=${2:-iso.iso}

mkdir -p build
pushd build
touch meta-data
cp -rfv $CLOUD_INIT user-data

mkisofs -output ci.iso -volid cidata -joliet -rock user-data meta-data
truncate -s "+$((20000*1024*1024))" disk.raw

${QEMU} -m 8096 -smp cores=2 \
        -nographic -cpu host \
        -serial mon:stdio \
        -rtc base=utc,clock=rt \
        -chardev socket,path=qga.sock,server,nowait,id=qga0 \
        -device virtio-serial \
        -device virtserialport,chardev=qga0,name=org.qemu.guest_agent.0 \
        -drive if=virtio,media=disk,file=disk.raw \
        -drive format=raw,media=cdrom,readonly=on,file=$ISO \
        -drive format=raw,media=cdrom,readonly=on,file=ci.iso \
        -boot d \
        -enable-kvm