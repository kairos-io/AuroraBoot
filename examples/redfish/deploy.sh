#!/bin/bash

# Example script for deploying an ISO to a server using RedFish

# Check if ISO path is provided
if [ -z "$1" ]; then
  echo "Usage: $0 <iso_path>"
  exit 1
fi

ISO_PATH=$1

# Deploy ISO to server using RedFish
auroraboot redfish deploy \
  --endpoint "https://example.com" \
  --username "admin" \
  --password "password" \
  --vendor "dmtf" \
  --verify-ssl true \
  --min-memory 4 \
  --min-cpus 2 \
  --required-features "UEFI" \
  --timeout 30m \
  "$ISO_PATH" 