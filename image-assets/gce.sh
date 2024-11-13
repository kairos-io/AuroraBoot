#!/bin/bash

# Transform a raw image disk to gce compatible
RAWIMAGE="$1"
OUT="${2:-$RAWIMAGE.gce.raw}"
cp -rf $RAWIMAGE $OUT

GB=$((1024*1024*1024))
size=$(qemu-img info -f raw --output json "$OUT" | gawk 'match($0, /"virtual-size": ([0-9]+),/, val) {print val[1]}')
# shellcheck disable=SC2004
ROUNDED_SIZE=$(echo "$size/$GB+1"|bc)
echo "Resizing raw image from \"$size\"MB to \"$ROUNDED_SIZE\"GB"
qemu-img resize -f raw "$OUT" "$ROUNDED_SIZE"G
echo "Compressing raw image $OUT to $OUT.tar.gz"
tar -c -z --format=oldgnu -f "$OUT".tar.gz $OUT
