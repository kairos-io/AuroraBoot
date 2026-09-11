package netboot

import (
	"context"
	"strconv"
	"time"

	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/netboot/booters"
	"github.com/kairos-io/netboot/server"
	"github.com/kairos-io/netboot/types"
)

// Server starts a netboot server which takes over and start to serve off booting in the same network
// It doesn't need any special configuration, however, requires binding to low ports.
func Server(ctx context.Context, kernel, cmdline string, address, httpPort, initrd string, nobind bool) error {

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
		internal.Log.Logger.Info().Str("subsystem", subsystem).Msg(msg)
	}

	loggerDebug := func(subsystem, msg string) {
		internal.Log.Logger.Debug().Str("subsystem", subsystem).Msg(msg)
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

	return serveUntilDone(ctx, s.Serve, s.Shutdown)
}

// shutdownGrace bounds how long a cancelled run waits for the server to close
// its listeners. Shutdown is a non-blocking send, so it is a no-op if the
// server has not finished binding yet; without a bound, waiting for a server
// that never got the signal would be the same hang again.
const shutdownGrace = 5 * time.Second

// serveUntilDone runs serve until it returns on its own or ctx is done,
// whichever happens first. serve blocks until a fatal error or a shutdown, so
// cancelling the context has to shut the server down explicitly: a context the
// server never reads is a context that cannot stop it.
func serveUntilDone(ctx context.Context, serve func() error, shutdown func()) error {
	errCh := make(chan error, 1)
	go func() { errCh <- serve() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdown()
		select {
		case <-errCh:
		case <-time.After(shutdownGrace):
		}
		return ctx.Err()
	}
}
