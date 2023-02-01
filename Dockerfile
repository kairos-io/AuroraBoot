FROM golang as builder
ADD . /work
RUN cd /work && \
    CGO_ENABLED=0 && \
    go build -o auroraboot

FROM opensuse/tumbleweed
RUN zypper in -y xorriso
COPY --from=builder /work/auroraboot /usr/bin/auroraboot

ENTRYPOINT ["/usr/bin/auroraboot"]