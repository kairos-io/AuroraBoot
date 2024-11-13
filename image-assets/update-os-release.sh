#!/bin/bash
# usage:
# docker run --rm -ti --entrypoint /update-os-release.sh \
# -v /etc:/workspace \ # mount the directory where your os-release is, this is by default in /etc but you can mount a different dir for testing
# -e OS_NAME=kairos-core-opensuse-leap \
# -e OS_VERSION=v2.2.0 \
# -e OS_ID="kairos" \
# -e OS_NAME=kairos-core-opensuse-leap \
# -e BUG_REPORT_URL="https://github.com/kairos-io/kairos/issues" \
# -e HOME_URL="https://github.com/kairos-io/kairos" \
# -e OS_REPO="quay.io/kairos/core-opensuse-leap" \
# -e OS_LABEL="latest" \
# -e GITHUB_REPO="kairos-io/kairos" \
# -e VARIANT="core" \
# -e FLAVOR="opensuse-leap"
# quay.io/kairos/osbuilder-tools:latest

set -ex

[ -f "/workspace/kairos-release" ] && sed -i -n '/KAIROS_/!p' /workspace/kairos-release
# Clean up old os-release just in case so we dont have stuff lying around
sed -i -n '/KAIROS_/!p' /workspace/os-release
envsubst >>/workspace/kairos-release < /kairos-release.tmpl

cat /workspace/kairos-release
