ARG VERSION=v0.400.2

FROM golang AS builder
ARG BINARY_VERSION=v0.0.0
WORKDIR /work
ADD go.mod .
ADD go.sum .
RUN go mod download
ADD . .
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${BINARY_VERSION}" -o auroraboot

FROM quay.io/kairos/osbuilder-tools:$VERSION
RUN zypper in -y qemu
COPY --from=builder /work/auroraboot /usr/bin/auroraboot
ENTRYPOINT ["/usr/bin/auroraboot"]
