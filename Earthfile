VERSION 0.7
ARG --global OSBUILDER_VERSION=v0.9.0

# renovate: datasource=github-releases depName=kairos-io/kairos
ARG IMAGE_VERSION=v3.2.1
ARG --global BASE_IMAGE=quay.io/kairos/ubuntu:24.04-core-amd64-generic-${IMAGE_VERSION}-uki

image:
    FROM DOCKERFILE --build-arg VERSION=$OSBUILDER_VERSION -f Dockerfile .
    RUN zypper in -y qemu

test-label:
    FROM alpine
    WORKDIR /test
    RUN apk add go docker jq
    ENV GOPATH=/go
    ENV FIXTURE_CONFIG=/test/tests/fixtures/raw_disk.yaml
    ARG LABEL
    COPY . .
    WITH DOCKER \
            --allow-privileged \
            --load auroraboot:latest=+image
        RUN pwd && ls -liah && go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo && /go/bin/ginkgo -r -p --randomize-all --procs 2 --fail-fast --timeout=2h --label-filter="$LABEL" --flake-attempts 3 ./...
    END

build-iso:
    FROM +image
    ARG BASE_IMAGE
    WORKDIR /build
    COPY e2e/assets/keys /keys
    # Extend the default cmdline to write everything to serial first :D
    RUN /usr/bin/auroraboot build-uki --output-dir /build -k /keys --output-type iso -x "console==ttyS0" $BASE_IMAGE
    SAVE ARTIFACT /build/*.iso kairos.iso AS LOCAL build/kairos.iso

test:
    FROM alpine
    WORKDIR /test
    RUN apk add go docker jq
    ENV GOPATH=/go
    ENV FIXTURE_CONFIG=/test/tests/fixtures/raw_disk.yaml
    COPY . .
    WITH DOCKER \
            --allow-privileged \
            --load auroraboot:latest=+image
        RUN pwd && ls -liah && go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo && /go/bin/ginkgo -r -p --randomize-all --procs 2 --fail-fast --timeout=2h --flake-attempts 3 ./...
    END
