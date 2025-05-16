package utils

import (
	"errors"
	"fmt"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/kairos-sdk/types"
	nbConstants "github.com/kairos-io/netboot/constants"
	"github.com/kairos-io/netboot/dhcp4"
	"net"
)

func ServeUkiPXE(log types.KairosLogger) error {
	log.Logger.Info().Msgf("Start pixiecore for UKI")
	dhcp, err := dhcp4.NewSnooperConn(fmt.Sprintf("%s:%d", "0.0.0.0", nbConstants.PortDHCP))
	if err != nil {
		return err
	}

	tftp, err := net.ListenPacket("udp", fmt.Sprintf("%s:%d", "0.0.0.0", nbConstants.PortTFTP))
	if err != nil {
		dhcp.Close()
		return err
	}
	pxe, err := net.ListenPacket("udp4", fmt.Sprintf("%s:%d", "0.0.0.0", nbConstants.PortPXE))
	if err != nil {
		dhcp.Close()
		tftp.Close()
		return err
	}
	http, err := net.Listen("tcp", fmt.Sprintf("%s:%d", "0.0.0.0", nbConstants.PortHTTP))
	if err != nil {
		dhcp.Close()
		tftp.Close()
		pxe.Close()
		return err
	}

	// 5 buffer slots, one for each goroutine, plus one for
	// Shutdown(). We only ever pull the first error out, but shutdown
	// will likely generate some spurious errors from the other
	// goroutines, and we want them to be able to dump them without
	// blocking.
	errs := make(chan error, 6)

	internal.Log.Logger.Debug().Str("subsystem", "Init").Msgf("Starting Pixiecore goroutines")

	// DHCP helps clients searching for ipxe servers find the right one
	// PXE I guess serves the pxe files???
	// TFTP serves the files to the clients
	// HTTP I guess its for plain http boot and htt requests

	go func() { errs <- serveDHCP(dhcp, log) }()
	//go func() { errs <- ret.servePXE(pxe) }()
	//go func() { errs <- ret.serveTFTP(tftp) }()
	err = <-errs
	dhcp.Close()
	tftp.Close()
	pxe.Close()
	http.Close()

	return err
}

func serveDHCP(conn *dhcp4.Conn, log types.KairosLogger) error {
	for {
		pkt, intf, err := conn.RecvDHCP()
		if err != nil {
			return fmt.Errorf("receiving DHCP packet: %s", err)
		}
		if intf == nil {
			return fmt.Errorf("received DHCP packet with no interface information (this is a violation of dhcp4.Conn's contract, please file a bug)")
		}

		if err = isBootDHCP(pkt); err != nil {
			log.Logger.Debug().Str("subsystem", "DHCP").Msgf("Ignoring packet from %s: %s", pkt.HardwareAddr, err)
			continue
		}

		// Machine should be booted.
		serverIP, err := interfaceIP(intf)
		if err != nil {
			log.Logger.Info().Str("subsystem", "DHCP").Msgf("Want to boot %s on %s, but couldn't get a source address: %s", pkt.HardwareAddr, intf.Name, err)
			continue
		}

		resp, err := offerDHCP(pkt, serverIP, log)
		if err != nil {
			log.Logger.Info().Str("subsystem", "DHCP").Msgf("Failed to construct ProxyDHCP offer for %s: %s", pkt.HardwareAddr, err)
			continue
		}

		if err = conn.SendDHCP(resp, intf); err != nil {
			log.Logger.Info().Str("subsystem", "DHCP").Msgf("Failed to send ProxyDHCP offer for %s: %s", pkt.HardwareAddr, err)
			continue
		}
	}
}

func isBootDHCP(pkt *dhcp4.Packet) error {
	if pkt.Type != dhcp4.MsgDiscover {
		return fmt.Errorf("packet is %s, not %s", pkt.Type, dhcp4.MsgDiscover)
	}

	if pkt.Options[dhcp4.OptClientSystem] == nil {
		return errors.New("not a PXE boot request (missing option 93)")
	}

	return nil
}

func interfaceIP(intf *net.Interface) (net.IP, error) {
	addrs, err := intf.Addrs()
	if err != nil {
		return nil, err
	}

	// Try to find an IPv4 address to use, in the following order:
	// global unicast (includes rfc1918), link-local unicast,
	// loopback.
	fs := [](func(net.IP) bool){
		net.IP.IsGlobalUnicast,
		net.IP.IsLinkLocalUnicast,
		net.IP.IsLoopback,
	}
	for _, f := range fs {
		for _, a := range addrs {
			ipaddr, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipaddr.IP.To4()
			if ip == nil {
				continue
			}
			if f(ip) {
				return ip, nil
			}
		}
	}

	return nil, errors.New("no usable unicast address configured on interface")
}

func offerDHCP(pkt *dhcp4.Packet, serverIP net.IP, log types.KairosLogger) (*dhcp4.Packet, error) {
	resp := &dhcp4.Packet{
		Type:          dhcp4.MsgOffer,
		TransactionID: pkt.TransactionID,
		Broadcast:     true,
		HardwareAddr:  pkt.HardwareAddr,
		RelayAddr:     pkt.RelayAddr,
		ServerAddr:    serverIP,
		Options:       make(dhcp4.Options),
	}
	resp.Options[dhcp4.OptServerIdentifier] = serverIP
	// says the server should identify itself as a PXEClient vendor
	// type, even though it's a server. Strange.
	resp.Options[dhcp4.OptVendorIdentifier] = []byte("PXEClient")
	if pkt.Options[dhcp4.OptUidGuidClientIdentifier] != nil {
		resp.Options[dhcp4.OptUidGuidClientIdentifier] = pkt.Options[dhcp4.OptUidGuidClientIdentifier]
	}

	resp.BootServerName = serverIP.String()
	resp.BootFilename = fmt.Sprintf("%s/booter.efi", serverIP)

	return resp, nil
}
