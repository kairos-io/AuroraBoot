FROM golang as builder
ADD . /work
RUN cd /work && \
    CGO_ENABLED=0 go build -o auroraboot

FROM quay.io/kairos/osbuilder-tools
COPY --from=builder /work/auroraboot /usr/bin/auroraboot

ENTRYPOINT ["/usr/bin/auroraboot"]