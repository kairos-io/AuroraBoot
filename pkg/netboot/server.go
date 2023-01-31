package netboot

import (
	"log"

	"go.universe.tf/netboot/out/ipxe"
	"go.universe.tf/netboot/pixiecore"
)

func Server(kernel, bootmsg, cmdline string, initrds []string) error {

	spec := &pixiecore.Spec{
		Kernel:  pixiecore.ID(kernel),
		Cmdline: cmdline,
		Message: bootmsg,
	}
	for _, initrd := range initrds {
		spec.Initrd = append(spec.Initrd, pixiecore.ID(initrd))
	}

	booter, err := pixiecore.StaticBooter(spec)
	if err != nil {
		return err
	}

	ipxeFw := map[pixiecore.Firmware][]byte{}
	ipxeFw[pixiecore.FirmwareX86PC] = ipxe.MustAsset("third_party/ipxe/src/bin/undionly.kpxe")
	ipxeFw[pixiecore.FirmwareEFI32] = ipxe.MustAsset("third_party/ipxe/src/bin-i386-efi/ipxe.efi")
	ipxeFw[pixiecore.FirmwareEFI64] = ipxe.MustAsset("third_party/ipxe/src/bin-x86_64-efi/ipxe.efi")
	ipxeFw[pixiecore.FirmwareEFIBC] = ipxe.MustAsset("third_party/ipxe/src/bin-x86_64-efi/ipxe.efi")
	ipxeFw[pixiecore.FirmwareX86Ipxe] = ipxe.MustAsset("third_party/ipxe/src/bin/ipxe.pxe")
	s := &pixiecore.Server{
		Ipxe:           ipxeFw,
		Log:            func(subsystem, msg string) { log.Printf("%s: %s\n", subsystem, msg) },
		HTTPPort:       80,
		HTTPStatusPort: 0,
		DHCPNoBind:     false,
		Address:        "0.0.0.0",
		UIAssetsDir:    "",
	}
	s.Booter = booter

	return s.Serve()
}
