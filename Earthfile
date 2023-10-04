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
    ARG LABEL
    COPY . .
    WITH DOCKER \
            --allow-privileged \
            --load auroraboot:latest=+image
        RUN go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo && /go/bin/ginkgo -r -p --randomize-all --procs 2 --fail-fast --label-filter="$LABEL" --flake-attempts 3 ./...
    END

test:
    FROM alpine
    WORKDIR /test
    RUN apk add go docker jq
    ENV GOPATH=/go
    COPY . .
    WITH DOCKER \
            --allow-privileged \
            --load auroraboot:latest=+image
        RUN go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo && /go/bin/ginkgo -r -p --randomize-all --procs 2 --fail-fast --flake-attempts 3 ./...
    END
