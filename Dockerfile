ARG VERSION=v0.400.3
ARG LUET_VERSION=0.35.5

FROM quay.io/luet/base:$LUET_VERSION AS luet

FROM golang AS builder
ARG BINARY_VERSION=v0.0.0
WORKDIR /work
ADD go.mod .
ADD go.sum .
RUN go mod download
ADD . .
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${BINARY_VERSION}" -o auroraboot

FROM quay.io/kairos/osbuilder-tools:$VERSION

COPY --from=luet /usr/bin/luet /usr/bin/luet
ENV LUET_NOLOCK=true
ENV TMPDIR=/tmp
ARG TARGETARCH
# copy both arches
COPY luet-arm64.yaml /tmp/luet-arm64.yaml
COPY luet-amd64.yaml /tmp/luet-amd64.yaml
# Set the default luet config to the current build arch
RUN mkdir -p /etc/luet/
RUN cp /tmp/luet-${TARGETARCH}.yaml /etc/luet/luet.yaml
## Uki artifacts, will be set under the /usr/kairos directory
RUN luet install -y system/systemd-boot

RUN zypper in -y qemu binutils
COPY --from=builder /work/auroraboot /usr/bin/auroraboot
ENTRYPOINT ["/usr/bin/auroraboot"]
