VERSION 0.6
ARG OSBUILDER_VERSION=v0.9.0

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
