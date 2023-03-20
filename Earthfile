VERSION 0.6

image:
    FROM DOCKERFILE -f Dockerfile .

test:
    FROM alpine
    WORKDIR /test
    RUN apk add go docker jq
    ENV GOPATH=/go
    COPY . .
    WITH DOCKER \
            --allow-privileged \
            --load auroraboot:latest=+image
        RUN go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo && /go/bin/ginkgo -r -p ./...
    END