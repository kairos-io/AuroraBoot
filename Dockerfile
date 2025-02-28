ARG FEDORA_VERSION=40
ARG LUET_VERSION=0.36.0

FROM quay.io/luet/base:$LUET_VERSION AS luet

FROM golang AS builder
ARG VERSION=v0.0.0
WORKDIR /work
ADD go.mod .
ADD go.sum .
RUN go mod download
ADD . .
ENV CGO_ENABLED=0
ENV VERSION=$VERSION
RUN go build -ldflags "-X main.version=${VERSION}" -o auroraboot

FROM fedora:$FEDORA_VERSION AS default
RUN dnf -y update
## ISO+ Arm image + Netboot + cloud images Build depedencies
RUN dnf in -y bc jq genisoimage docker sudo parted e2fsprogs erofs-utils binutils curl util-linux udev rsync \
    dosfstools mtools xorriso lvm2 zstd sbsigntools squashfs-tools kpartx grub2

COPY --from=luet /usr/bin/luet /usr/bin/luet
ENV LUET_NOLOCK=true
ENV TMPDIR=/tmp
ARG TARGETARCH
# copy both arches
COPY image-assets/luet-arm64.yaml /tmp/luet-arm64.yaml
COPY image-assets/luet-amd64.yaml /tmp/luet-amd64.yaml
# Set the default luet config to the current build arch
RUN mkdir -p /etc/luet/
RUN cp /tmp/luet-${TARGETARCH}.yaml /etc/luet/luet.yaml
## Uki artifacts, will be set under the /usr/kairos directory
# `luet repo update` fails with `/usr/bin/unpigz: invalid argument` on arm for some reason without this option:
# https://github.com/containerd/containerd/blob/7c3aca7a610df76212171d200ca3811ff6096eb8/archive/compression/compression.go#L50
ENV CONTAINERD_DISABLE_PIGZ=1
RUN luet repo update
RUN luet install -y system/systemd-boot

# Install the grub2-efi-image package to get the unsigned shim and the unsigned grub.efi
# TODO: Remove this. Only used by alpine+arch. It should fallback to not install the shim if on alpine and install the usigned grub.efi
# Alpine doesnt ship a shim so we can skip it directly and install the grub in the shim place.
RUN luet install -y livecd/grub2-efi-image --system-target /efi

## RPI64
RUN luet install -y firmware/u-boot-rpi64 firmware/raspberrypi-firmware firmware/raspberrypi-firmware-config firmware/raspberrypi-firmware-dt --system-target /rpi/

## RPI5
RUN luet install -y firmware/rpi5 --system-target /rpi5/

## PineBook64 Pro
RUN luet install -y arm-vendor-blob/u-boot-rockchip --system-target /pinebookpro/u-boot

## Odroid fw
RUN luet install -y firmware/odroid-c2 --system-target /firmware/odroid-c2

## RAW images for arm64
# Luet will install this artifacts from the current arch repo, so in x86 it will
# get them from the x86 repo and we want it to do it from the arm64 repo, even on x86
# so we use the arm64 luet config and use that to install those on x86
# This is being used by the prepare_arm_images.sh and build-arch-image.sh scripts
# TODO: Remove this. Only used for orin image. It should be built in the RAW image generation directly so we cna drop this
RUN luet install --config /tmp/luet-arm64.yaml -y static/grub-efi --system-target /arm/raw/grubefi
RUN luet install --config /tmp/luet-arm64.yaml -y static/grub-config --system-target /arm/raw/grubconfig
RUN luet install --config /tmp/luet-arm64.yaml -y static/grub-artifacts --system-target /arm/raw/grubartifacts

# kairos-agent so we can use the pull-image
# TODO: What? I cant see where this is used anywhere? Check why its here? Its like 35Mb on nothingness if not used?
RUN luet install -y system/kairos-agent

# remove luet tmp files. Side effect of setting the system-target is that it treats it as a root fs
# so temporal files are stored in each dir
RUN rm -Rf /grub2/var/tmp
RUN rm -Rf /grub2/var/cache
RUN rm -Rf /efi/var/tmp
RUN rm -Rf /efi/var/cache
RUN rm -Rf /rpi/var/tmp
RUN rm -Rf /rpi/var/cache
RUN rm -Rf /pinebookpro/u-boot/var/tmp
RUN rm -Rf /pinebookpro/u-boot/var/cache
RUN rm -Rf /firmware/odroid-c2/var/tmp
RUN rm -Rf /firmware/odroid-c2/var/cache
RUN rm -Rf /arm/raw/grubefi/var/tmp
RUN rm -Rf /arm/raw/grubefi/var/cache
RUN rm -Rf /arm/raw/grubconfig/var/tmp
RUN rm -Rf /arm/raw/grubconfig/var/cache
RUN rm -Rf /arm/raw/grubartifacts/var/tmp
RUN rm -Rf /arm/raw/grubartifacts/var/cache
# Remove the var dir if empty
RUN rm -d /grub2/var || true
RUN rm -d /efi/var || true
RUN rm -d /rpi/var || true
RUN rm -d /pinebookpro/u-boot/var || true
RUN rm -d /firmware/odroid-c2/var || true
RUN rm -d /arm/raw/grubefi/var || true
RUN rm -d /arm/raw/grubconfig/var || true
RUN rm -d /arm/raw/grubartifacts/var || true

# ARM helpers
COPY ./image-assets/prepare_nvidia_orin_images.sh /prepare_nvidia_orin_images.sh

COPY --from=builder /work/auroraboot /usr/bin/auroraboot

ENTRYPOINT ["/usr/bin/auroraboot"]
