#!/bin/bash

# This bundle needs to run after-install as it consumes assets from the LiveCD, which is not accessible at the first boot (there is no live-cd in)
persistent=$(blkid -L COS_PERSISTENT)
mkdir /tmp/persistent
mount $persistent /tmp/persistent
mkdir -p /usr/local/.state/var-lib-rancher.bind/k3s/agent/images/
cp ./assets/k3s-airgap-images-amd64.tar /usr/local/.state/var-lib-rancher.bind/k3s/agent/images/
