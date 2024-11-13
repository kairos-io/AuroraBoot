#!/bin/bash
# Extracts squashfs, kernel, initrd and generates a ipxe template script

ISO=$1
OUTPUT_NAME=$2
ARTIFACT_NAME=$(basename $OUTPUT_NAME)

isoinfo -x /rootfs.squashfs -R -i $ISO > $OUTPUT_NAME.squashfs
isoinfo -x /boot/kernel -R -i $ISO > $OUTPUT_NAME-kernel
isoinfo -x /boot/initrd -R -i $ISO > $OUTPUT_NAME-initrd

URL=${URL:-https://github.com/kairos-io/kairos/releases/download}

cat > $OUTPUT_NAME.ipxe << EOF
#!ipxe
set url ${URL}/
set kernel $ARTIFACT_NAME-kernel
set initrd $ARTIFACT_NAME-initrd
set rootfs $ARTIFACT_NAME.squashfs
# set config https://example.com/machine-config
# set cmdline extra.values=1
kernel \${url}/\${kernel} initrd=\${initrd} ip=dhcp rd.cos.disable root=live:\${url}/\${rootfs} netboot install-mode config_url=\${config} console=tty1 console=ttyS0 \${cmdline}
initrd \${url}/\${initrd}
boot
EOF