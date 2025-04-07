# RedFish Deployment Examples

This directory contains examples for deploying ISOs to servers using the RedFish protocol.

## Command Line Usage

```bash
# Deploy an ISO to a server using the generic RedFish client
auroraboot redfish deploy --endpoint https://example.com --username admin --password password --vendor generic --verify-ssl true path/to/iso

# Deploy an ISO to a SuperMicro server
auroraboot redfish deploy --endpoint https://supermicro.example.com --username admin --password password --vendor supermicro --verify-ssl true path/to/iso

# Deploy an ISO to an HPE iLO server
auroraboot redfish deploy --endpoint https://ilo.example.com --username admin --password password --vendor ilo --verify-ssl true path/to/iso

# Deploy an ISO to a PiKVM server (DMTF implementation)
auroraboot redfish deploy --endpoint https://pikvm.example.com --username admin --password password --vendor dmtf --verify-ssl true path/to/iso
```

## Docker Usage

```bash
# Deploy an ISO to a server using the generic RedFish client
docker run --rm -v /path/to/iso:/iso quay.io/kairos/auroraboot redfish deploy --endpoint https://example.com --username admin --password password --vendor generic --verify-ssl true /iso

# Deploy an ISO to a SuperMicro server
docker run --rm -v /path/to/iso:/iso quay.io/kairos/auroraboot redfish deploy --endpoint https://supermicro.example.com --username admin --password password --vendor supermicro --verify-ssl true /iso

# Deploy an ISO to an HPE iLO server
docker run --rm -v /path/to/iso:/iso quay.io/kairos/auroraboot redfish deploy --endpoint https://ilo.example.com --username admin --password password --vendor ilo --verify-ssl true /iso

# Deploy an ISO to a PiKVM server (DMTF implementation)
docker run --rm -v /path/to/iso:/iso quay.io/kairos/auroraboot redfish deploy --endpoint https://pikvm.example.com --username admin --password password --vendor dmtf --verify-ssl true /iso
```

## Available Vendors

- `generic`: Generic RedFish implementation
- `supermicro`: SuperMicro servers
- `ilo`: HPE iLO servers
- `dmtf`: DMTF-compliant servers (e.g., PiKVM)

## Additional Options

- `--min-memory`: Minimum required memory in GiB (default: 4)
- `--min-cpus`: Minimum required CPUs (default: 2)
- `--required-features`: Required hardware features (default: UEFI)
- `--timeout`: Operation timeout (default: 30m) 