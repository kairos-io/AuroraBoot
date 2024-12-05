VERSION 0.7

# renovate: datasource=docker depName=golang
ARG GO_VERSION=1.23
ARG --global GO_IMAGE=$GO_VERSION-bookworm

# renovate: datasource=github-releases depName=kairos-io/kairos
ARG IMAGE_VERSION=v3.2.1
ARG --global BASE_IMAGE=quay.io/kairos/ubuntu:24.04-core-amd64-generic-${IMAGE_VERSION}-uki

version:
  FROM alpine
  RUN apk update && apk add git

  COPY . .
  RUN --no-cache git describe --always --tags --dirty > VERSION
  SAVE ARTIFACT VERSION VERSION

image:
  FROM +version
  ARG VERSION=$(cat VERSION)
  FROM DOCKERFILE --build-arg VERSION=$VERSION -f Dockerfile .
  SAVE IMAGE quay.io/kairos/auroraboot:$VERSION

build-iso:
    FROM +image
    ARG BASE_IMAGE
    WORKDIR /build
    COPY e2e/assets/keys /keys
    # Extend the default cmdline to write everything to serial first :D
    RUN /usr/bin/auroraboot build-uki --output-dir /build -k /keys --output-type iso -x "console=ttyS0" $BASE_IMAGE
    SAVE ARTIFACT /build/*.iso kairos.iso AS LOCAL build/kairos.iso

go-deps:
    FROM golang:$GO_IMAGE
    WORKDIR /build
    COPY go.mod go.sum . # This will make the go mod download able to be cached as long as it hasnt change
    RUN go mod download
    SAVE ARTIFACT go.mod AS LOCAL go.mod
    SAVE ARTIFACT go.sum AS LOCAL go.sum

test-bootable:
    FROM +go-deps
    WORKDIR /build
    RUN . /etc/os-release && echo "deb http://deb.debian.org/debian $VERSION_CODENAME-backports main contrib non-free" > /etc/apt/sources.list.d/backports.list
    RUN apt update
    RUN apt install -y qemu-system-x86 qemu-utils git swtpm && apt clean
    COPY . .
    COPY +build-iso/kairos.iso kairos.iso
    ARG ISO=/build/kairos.iso
    ARG FIRMWARE=/usr/share/OVMF/OVMF_CODE.fd
    ARG USE_QEMU=true
    ARG MEMORY=4000
    ARG CPUS=2
    ARG CREATE_VM=true
    RUN date
    RUN go run github.com/onsi/ginkgo/v2/ginkgo run --label-filter "bootable" -v --fail-fast -r ./e2e

last-commit-packages:
    FROM quay.io/skopeo/stable
    RUN dnf install -y jq
    WORKDIR build
    ENV jqQuery='.Tags | map(select(. | contains("-repository.yaml"))) | sort_by(. | sub("v";"") | sub("-repository.yaml";"") | sub("-git.*";"") | .[0:12] | tonumber) | .[-1]'
    RUN skopeo list-tags docker://quay.io/kairos/packages | jq -rc "${jqQuery}" >  REPO_AMD64
    RUN skopeo list-tags docker://quay.io/kairos/packages-arm64 | jq -rc "${jqQuery}" > REPO_ARM64
    SAVE ARTIFACT REPO_AMD64 REPO_AMD64
    SAVE ARTIFACT REPO_ARM64 REPO_ARM64

bump-repositories:
    FROM mikefarah/yq
    WORKDIR build
    COPY +last-commit-packages/REPO_AMD64 REPO_AMD64
    COPY +last-commit-packages/REPO_ARM64 REPO_ARM64
    ARG REPO_AMD64=$(cat REPO_AMD64)
    ARG REPO_ARM64=$(cat REPO_ARM64)
    COPY image-assets/luet-amd64.yaml luet-amd64.yaml
    COPY image-assets/luet-arm64.yaml luet-arm64.yaml
    RUN yq eval ".repositories[0] |= . * { \"reference\": \"${REPO_AMD64}\" }" -i luet-amd64.yaml
    RUN yq eval ".repositories[0] |= . * { \"reference\": \"${REPO_ARM64}\" }" -i luet-arm64.yaml
    SAVE ARTIFACT luet-arm64.yaml AS LOCAL image-assets/luet-arm64.yaml
    SAVE ARTIFACT luet-amd64.yaml AS LOCAL image-assets/luet-amd64.yaml
