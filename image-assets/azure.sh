#!/bin/bash

set -ex
# Transform a raw image disk to azure vhd
RAWIMAGE="$1"
# The final name of the image
OUT="${2:-$RAWIMAGE.vhd}"

# Check if OUT has .vhd extension, if not add it
if [[ "$OUT" != *.vhd ]]; then
  OUT="$OUT.vhd"
fi


MB=$((1024*1024))
size=$(qemu-img info -f raw --output json "$RAWIMAGE" | gawk 'match($0, /"virtual-size": ([0-9]+),/, val) {print val[1];exit}')
# shellcheck disable=SC2004
ROUNDED_SIZE=$(((($size+$MB-1)/$MB)*$MB))
echo "Resizing raw image from $size MB to $ROUNDED_SIZE MB"
qemu-img resize -f raw "$RAWIMAGE" $ROUNDED_SIZE
echo "Converting $RAWIMAGE to Azure VHD format"
qemu-img convert -f raw -o subformat=fixed,force_size -O vpc "$RAWIMAGE" "$OUT"
echo "Done"
rm -rf "$RAWIMAGE"