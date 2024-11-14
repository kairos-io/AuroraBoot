#!/bin/bash

# Transform a raw image disk to azure vhd
RAWIMAGE="$1"
VHDDISK="${2:-disk.vhd}"
cp -rf $RAWIMAGE $VHDDISK.work

MB=$((1024*1024))
size=$(qemu-img info -f raw --output json "$RAWIMAGE" | gawk 'match($0, /"virtual-size": ([0-9]+),/, val) {print val[1]}')
# shellcheck disable=SC2004
ROUNDED_SIZE=$(((($size+$MB-1)/$MB)*$MB))
echo "Resizing raw image to $ROUNDED_SIZE"
qemu-img resize -f raw "$VHDDISK.work" $ROUNDED_SIZE
echo "Converting $RAWIMAGE to $VHDDISK"
qemu-img convert -f raw -o subformat=fixed,force_size -O vpc "$VHDDISK.work" "$VHDDISK"
echo "Done"
rm -rf "$VHDDISK.work"