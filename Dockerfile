ARG VERSION=v0.9.0

FROM golang as builder
ADD . /work
RUN cd /work && \
    CGO_ENABLED=0 go build -o auroraboot

FROM quay.io/kairos/osbuilder-tools:$VERSION
COPY --from=builder /work/auroraboot /usr/bin/auroraboot
RUN zypper in -y qemu
ENTRYPOINT ["/usr/bin/auroraboot"]