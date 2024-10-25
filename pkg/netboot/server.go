package netboot

import (
	"strconv"

	"github.com/kairos-io/AuroraBoot/internal"
	"go.universe.tf/netboot/out/ipxe"
	"go.universe.tf/netboot/pixiecore"
)

// Server starts a netboot server which takes over and start to serve off booting in the same network
// It doesn't need any special configuration, however, requires binding to low ports.
func Server(kernel, cmdline string, address, httpPort, initrd string, nobind bool) error {

	spec := &pixiecore.Spec{
		Kernel:  pixiecore.ID(kernel),
		Cmdline: cmdline,
		Initrd:  []pixiecore.ID{pixiecore.ID(initrd)},
	}

	booter, err := pixiecore.StaticBooter(spec)
	if err != nil {
		return err
	}

	port, err := strconv.Atoi(httpPort)
	if err != nil {
		return err
	}

	logger := func(subsystem, msg string) {
		internal.Log.Logger.Info().Str("subsystem", subsystem).Msg(msg)
	}

	loggerDebug := func(subsystem, msg string) {
		internal.Log.Logger.Debug().Str("subsystem", subsystem).Msg(msg)
	}

	ipxeFw := map[pixiecore.Firmware][]byte{}
	ipxeFw[pixiecore.FirmwareX86PC] = ipxe.MustAsset("third_party/ipxe/src/bin/undionly.kpxe")
	ipxeFw[pixiecore.FirmwareEFI32] = ipxe.MustAsset("third_party/ipxe/src/bin-i386-efi/ipxe.efi")
	ipxeFw[pixiecore.FirmwareEFI64] = ipxe.MustAsset("third_party/ipxe/src/bin-x86_64-efi/ipxe.efi")
	ipxeFw[pixiecore.FirmwareEFIBC] = ipxe.MustAsset("third_party/ipxe/src/bin-x86_64-efi/ipxe.efi")
	ipxeFw[pixiecore.FirmwareX86Ipxe] = ipxe.MustAsset("third_party/ipxe/src/bin/ipxe.pxe")
	s := &pixiecore.Server{
		Ipxe:           ipxeFw,
		Log:            logger,
		Debug:          loggerDebug,
		HTTPPort:       port,
		HTTPStatusPort: 0,
		DHCPNoBind:     nobind,
		Address:        address,
		UIAssetsDir:    "",
	}
	s.Booter = booter

	return s.Serve()
}
