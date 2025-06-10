# RedFish Deployment (Experimental)

AuroraBoot now includes experimental support for deploying ISOs to servers using the RedFish protocol. This feature allows you to remotely deploy ISOs to servers that support the RedFish API, including various hardware vendors.

**Note: This is an experimental feature and requires testing with actual hardware. Use with caution in production environments.**

## Command Line Usage

```bash
# Deploy an ISO to a PiKVM server (DMTF implementation)
auroraboot redfish deploy --endpoint https://pikvm.example.com --username admin --password password --vendor dmtf --verify-ssl true path/to/iso
```

## Docker Usage

```bash
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