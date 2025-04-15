package netboot

import (
	"strconv"

	"github.com/kairos-io/AuroraBoot/internal/log"
	"github.com/kairos-io/netboot/booters"
	"github.com/kairos-io/netboot/server"
	"github.com/kairos-io/netboot/types"
)

// Server starts a netboot server which takes over and start to serve off booting in the same network
// It doesn't need any special configuration, however, requires binding to low ports.
func Server(kernel, cmdline string, address, httpPort, initrd string, nobind bool) error {

	spec := &types.Spec{
		Kernel:  types.ID(kernel),
		Cmdline: cmdline,
		Initrd:  []types.ID{types.ID(initrd)},
	}

	booter, err := booters.StaticBooter(spec)
	if err != nil {
		return err
	}

	port, err := strconv.Atoi(httpPort)
	if err != nil {
		return err
	}

	logger := func(subsystem, msg string) {
		log.Log.Logger.Info().Str("subsystem", subsystem).Msg(msg)
	}

	loggerDebug := func(subsystem, msg string) {
		log.Log.Logger.Debug().Str("subsystem", subsystem).Msg(msg)
	}

	s := &server.Server{
		Log:        logger,
		Debug:      loggerDebug,
		HTTPPort:   port,
		DHCPNoBind: nobind,
		Address:    address,
	}

	// sets the default firmwares for booting
	s.SetDefaultFirmwares()

	s.Booter = booter

	return s.Serve()
}
