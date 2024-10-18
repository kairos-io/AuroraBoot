#!/bin/bash

set -e

#--rm -ti quay.io/kairos/auroraboot \
CGO_ENABLED=0 go build -o build/auroraboot . && \
docker run -v "$PWD"/config.yaml:/config.yaml \
  -v "$PWD"/build:/tmp/auroraboot \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $PWD/../enki/build/enki:/usr/bin/enki \
  -v $PWD/build/auroraboot:/usr/bin/auroraboot \
  --rm -ti myauroraboot:latest \
  --debug \
  --set container_image=docker://quay.io/kairos/ubuntu:24.04-core-amd64-generic-v3.2.1 \
  --set "disable_http_server=true" \
  --set "disable_netboot=true" \
  --cloud-config /config.yaml \
  --set "state_dir=/tmp/auroraboot"
