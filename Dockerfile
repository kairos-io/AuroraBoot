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
    dosfstools mtools xorriso lvm2 zstd sbsigntools squashfs-tools kpartx grub2 openssl

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
## Each arch has its own systemd-boot artifacts, we should ship both in both images for multi-arch build support
RUN luet install --config /tmp/luet-arm64.yaml -y system/systemd-boot --system-target /arm/systemd-boot
RUN luet install --config /tmp/luet-amd64.yaml -y system/systemd-boot --system-target /amd/systemd-boot

## RPI64
## Both arches have the same package files so no matter the arch here.
RUN luet install -y firmware/u-boot-rpi64 firmware/rpi --system-target /arm/rpi/

## PineBook64 Pro
RUN luet install -y arm-vendor-blob/u-boot-rockchip --system-target /arm/pinebookpro/

## Odroid fw
RUN luet install -y firmware/odroid-c2 --system-target /arm/odroid-c2

## RAW images for arm64

# Orin uses these artifacts
RUN luet install --config /tmp/luet-arm64.yaml -y static/grub-efi --system-target /arm/raw/grubefi
RUN luet install --config /tmp/luet-arm64.yaml -y static/grub-config --system-target /arm/raw/grubconfig
# Orin uses these artifacts. Alpine uses these artifacts for fallback efi values
RUN luet install --config /tmp/luet-arm64.yaml -y static/grub-artifacts --system-target /arm/raw/grubartifacts
# You can build amd64 raw images for alpine so....we need this in both images
RUN luet install --config /tmp/luet-amd64.yaml -y static/grub-artifacts --system-target /amd/raw/grubartifacts

# kairos-agent so we can use the pull-image
# TODO: What? I cant see where this is used anywhere? Check why its here? Its like 35Mb on nothingness if not used?
RUN luet install -y system/kairos-agent

# remove luet tmp files. Side effect of setting the system-target is that it treats it as a root fs
# so temporal files are stored in each dir
RUN rm -Rf /arm/systemd-boot/var/tmp
RUN rm -Rf /amd/systemd-boot/var/tmp
RUN rm -Rf /arm/systemd-boot/var/cache
RUN rm -Rf /amd/systemd-boot/var/cache
RUN rm -Rf /arm/rpi/var/tmp
RUN rm -Rf /arm/rpi/var/cache
RUN rm -Rf /arm/pinebookpro/var/tmp
RUN rm -Rf /arm/pinebookpro/var/cache
RUN rm -Rf /arm/odroid-c2/var/tmp
RUN rm -Rf /arm/odroid-c2/var/cache
RUN rm -Rf /arm/raw/grubconfig/var/tmp
RUN rm -Rf /arm/raw/grubconfig/var/cache
RUN rm -Rf /arm/raw/grubartifacts/var/tmp
RUN rm -Rf /amd/raw/grubartifacts/var/tmp
RUN rm -Rf /arm/raw/grubartifacts/var/cache
RUN rm -Rf /amd/raw/grubartifacts/var/cache
RUN rm -Rf /arm/raw/grubefi/var/tmp
RUN rm -Rf /arm/raw/grubefi/var/cache
# Remove the var dir if empty
RUN rm -d /arm/systemd-boot/var || true
RUN rm -d /amd/systemd-boot/var || true
RUN rm -d /arm/rpi/var || true
RUN rm -d /arm/pinebookpro/var || true
RUN rm -d /arm/odroid-c2/var || true
RUN rm -d /arm/raw/grubconfig/var || true
RUN rm -d /arm/raw/grubartifacts/var || true
RUN rm -d /amd/raw/grubartifacts/var || true
RUN rm -d /arm/raw/grubefi/var || true

# ARM helpers
COPY ./image-assets/prepare_nvidia_orin_images.sh /prepare_nvidia_orin_images.sh

COPY ./image-assets/defaults.yaml /defaults.yaml

ENV BUILDKIT_PROGRESS=plain
RUN dnf -y install dnf-plugins-core && dnf-3 config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo && dnf install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin

COPY --from=builder /work/auroraboot /usr/bin/auroraboot

ENTRYPOINT ["/usr/bin/auroraboot"]
