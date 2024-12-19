#!/bin/bash

set -ex
# Transform a raw image disk to gce compatible
RAWIMAGE="$1"
# The final name of the image
# This is the one we have control over as the raw image inside the tarball will be named disk.raw as required by GCE
OUT="${2:-$RAWIMAGE.gce.tar.gz}"

# Check if OUT has .tar.gz extension, if not add it
if [[ "$OUT" != *.tar.gz ]]; then
  OUT="$OUT.tar.gz"
fi

GB=$((1024*1024*1024))
MB=$((1024*1024))
size=$(qemu-img info -f raw --output json "$RAWIMAGE" | gawk 'match($0, /"virtual-size": ([0-9]+),/, val) {print val[1];exit}')
# shellcheck disable=SC2004
ROUNDED_SIZE=$(echo "$size/$GB+1"|bc)
CURRENT_SIZE=$(echo "$size/$MB"|bc)
echo "Resizing raw image from \"$size\"MB to \"$ROUNDED_SIZE\"GB"
qemu-img resize -f raw "$RAWIMAGE" "$ROUNDED_SIZE"G
echo "Renaming image to disk.raw as required by GCE"
mv "$RAWIMAGE" disk.raw
echo "Compressing raw image disk.raw to $OUT"
tar -c -z --format=oldgnu -f "$OUT" disk.raw
