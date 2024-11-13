#!/bin/bash
# docker run --entrypoint /add-cloud-init.sh -v $PWD:/work -ti --rm test https://github.com/kairos-io/kairos/releases/download/v1.1.2/kairos-alpine-v1.1.2.iso /work/test.iso /work/config.yaml

set -ex

ISO=$1
OUT=$2
CONFIG=$3

case ${ISO} in
http*)
    curl -L "${ISO}" -o in.iso
    ISO=in.iso
    ;;
esac

# Needs xorriso >=1.5.4
xorriso -indev $ISO -outdev $OUT -map $CONFIG /config.yaml -boot_image any replay