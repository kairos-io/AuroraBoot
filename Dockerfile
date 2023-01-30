ARG LDFLAGS=-s -w
FROM golang:alpine as builder
ENV LDFLAGS=$LDFLAGS
ADD . /work
RUN cd /work && \
    CGO_ENABLED=0 && \
    go build -ldflags="$LDFLAGS" -o auroraboot

FROM quay.io/pixiecore/pixiecore
COPY --from=builder /work/auroraboot /usr/bin/auroraboot

ENTRYPOINT ["/usr/bin/auroraboot"]