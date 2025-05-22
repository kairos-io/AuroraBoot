package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/constants"
	"github.com/kairos-io/kairos-sdk/types"
	nbConstants "github.com/kairos-io/netboot/constants"
	"github.com/kairos-io/netboot/dhcp4"
	"github.com/kairos-io/netboot/tftp"
	"golang.org/x/net/ipv4"
)

func ServeUkiPXE(keydir, isoFile string, log types.KairosLogger) error {
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
	// 5 buffer slots, one for each goroutine, plus one for
	// Shutdown(). We only ever pull the first error out, but shutdown
	// will likely generate some spurious errors from the other
	// goroutines, and we want them to be able to dump them without
	// blocking.
	errs := make(chan error, 6)

	internal.Log.Logger.Debug().Str("subsystem", "Init").Msgf("Starting Pixiecore goroutines")

	// DHCP helps clients searching for iPXE servers find the right one
	// PXE serves the PXE request to point to the TFTP server
	// TFTP serves the files to the clients
	// HTTP is used for plain HTTP boot and HTTP requests

	go func() { errs <- serveDHCP(dhcp, log) }()
	go func() { errs <- servePXE(pxe, log) }()
	go func() { errs <- serveTFTP(tftp, log) }()
	go func() { errs <- serveHTTP(keydir, isoFile, log) }()
	err = <-errs
	dhcp.Close()
	tftp.Close()
	pxe.Close()

	return err
}

func serveDHCP(conn *dhcp4.Conn, log types.KairosLogger) error {
	log.Logger.Info().Str("subsystem", "DHCP").Msgf("Listening for requests on :%d", nbConstants.PortDHCP)
	for {
		pkt, intf, err := conn.RecvDHCP()
		if err != nil {
			return fmt.Errorf("receiving DHCP packet: %s", err)
		}
		if intf == nil {
			return fmt.Errorf("received DHCP packet with no interface information (this is a violation of dhcp4.Conn's contract, please file a bug)")
		}
		log.Logger.Debug().Str("subsystem", "DHCP").Msgf("Received DHCP packet from %s on %s", pkt.HardwareAddr, intf.Name)

		if err = isBootDHCP(pkt); err != nil {
			log.Logger.Debug().Str("subsystem", "DHCP").Msgf("Ignoring packet from %s: %s", pkt.HardwareAddr, err)
			continue
		}

		// Machine should be booted.
		serverIP, err := interfaceIP(intf)
		if err != nil {
			log.Logger.Debug().Str("subsystem", "DHCP").Msgf("Want to boot %s on %s, but couldn't get a source address: %s", pkt.HardwareAddr, intf.Name, err)
			continue
		}

		resp, err := offerDhcpPackage(pkt, dhcp4.MsgOffer, serverIP, log)
		if err != nil {
			log.Logger.Debug().Str("subsystem", "DHCP").Msgf("Failed to construct ProxyDHCP offer for %s: %s", pkt.HardwareAddr, err)
			continue
		}

		if err = conn.SendDHCP(resp, intf); err != nil {
			log.Logger.Debug().Str("subsystem", "DHCP").Msgf("Failed to send ProxyDHCP offer for %s: %s", pkt.HardwareAddr, err)
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
	fs := []func(net.IP) bool{
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

func servePXE(conn net.PacketConn, log types.KairosLogger) error {
	log.Logger.Info().Str("subsystem", "PXE").Msgf("Listening for requests on :%d", nbConstants.PortPXE)
	buf := make([]byte, 1024)
	l := ipv4.NewPacketConn(conn)
	if err := l.SetControlMessage(ipv4.FlagInterface, true); err != nil {
		return fmt.Errorf("couldn't get interface metadata on PXE port: %s", err)
	}

	for {
		n, msg, addr, err := l.ReadFrom(buf)
		if err != nil {
			return fmt.Errorf("receiving packet: %s", err)
		}

		pkt, err := dhcp4.Unmarshal(buf[:n])
		if err != nil {
			log.Logger.Debug().Str("subsystem", "PXE").Msgf("Packet from %s is not a DHCP packet: %s", addr, err)
			continue
		}

		if err = isBootDHCP(pkt); err != nil {
			log.Logger.Debug().Str("subsystem", "PXE").Msgf("Ignoring packet from %s (%s): %s", pkt.HardwareAddr, addr, err)
			continue
		}

		intf, err := net.InterfaceByIndex(msg.IfIndex)
		if err != nil {
			log.Logger.Debug().Str("subsystem", "PXE").Msgf("Couldn't get information about local network interface %d: %s", msg.IfIndex, err)
			continue
		}

		serverIP, err := interfaceIP(intf)
		if err != nil {
			log.Logger.Debug().Str("subsystem", "PXE").Msgf("Want to boot %s (%s) on %s, but couldn't get a source address: %s", pkt.HardwareAddr, addr, intf.Name, err)
			continue
		}

		resp, err := offerDhcpPackage(pkt, dhcp4.MsgAck, serverIP, log)
		if err != nil {
			log.Logger.Debug().Str("subsystem", "PXE").Msgf("Failed to construct PXE offer for %s (%s): %s", pkt.HardwareAddr, addr, err)
			continue
		}

		bs, err := resp.Marshal()
		if err != nil {
			log.Logger.Debug().Str("subsystem", "PXE").Msgf("Failed to marshal PXE offer for %s (%s): %s", pkt.HardwareAddr, addr, err)
			continue
		}

		if _, err := l.WriteTo(bs, &ipv4.ControlMessage{
			IfIndex: msg.IfIndex,
		}, addr); err != nil {
			log.Logger.Debug().Str("subsystem", "PXE").Msgf("Failed to send PXE response to %s (%s): %s", pkt.HardwareAddr, addr, err)
		}
	}
}

func offerDhcpPackage(pkt *dhcp4.Packet, t dhcp4.MessageType, serverIP net.IP, log types.KairosLogger) (resp *dhcp4.Packet, err error) {
	resp = &dhcp4.Packet{
		Type:           t,
		TransactionID:  pkt.TransactionID,
		HardwareAddr:   pkt.HardwareAddr,
		ClientAddr:     pkt.ClientAddr,
		RelayAddr:      pkt.RelayAddr,
		ServerAddr:     serverIP,
		BootServerName: serverIP.String(),
		BootFilename:   fmt.Sprintf("%s/booter.efi", serverIP),
		Options: dhcp4.Options{
			dhcp4.OptServerIdentifier: serverIP,
		},
	}
	if pkt.Options[dhcp4.OptUidGuidClientIdentifier] != nil {
		resp.Options[dhcp4.OptUidGuidClientIdentifier] = pkt.Options[dhcp4.OptUidGuidClientIdentifier]
	}

	if strings.Contains(strings.ToLower(string(pkt.Options[dhcp4.OptVendorIdentifier])), "httpclient") {
		log.Logger.Debug().Str("subsystem", "PXE").Msgf("Client %s is a HTTPClient", pkt.HardwareAddr)
		resp.Options[dhcp4.OptVendorIdentifier] = []byte("HTTPClient")
		resp.BootFilename = fmt.Sprintf("http://%s/booter.efi", serverIP)
	} else {
		log.Logger.Debug().Str("subsystem", "PXE").Msgf("Client %s is a PXEClient", pkt.HardwareAddr)
		resp.Options[dhcp4.OptVendorIdentifier] = []byte("PXEClient")
	}

	log.Logger.Debug().Str("subsystem", "PXE").Str("VendorIdentifier", string(pkt.Options[dhcp4.OptVendorIdentifier])).Msg("Processed Vendor Identifier")

	log.Logger.Debug().Str("BootServerName", resp.BootServerName).Str("BootFilename", resp.BootFilename).Msgf("Sending ProxyDHCP offer to %s on %s", pkt.HardwareAddr, serverIP)
	return resp, nil
}

func serveTFTP(conn net.PacketConn, log types.KairosLogger) error {
	log.Logger.Info().Str("subsystem", "TFTP").Msgf("Listening for requests on :%d", nbConstants.PortTFTP)
	ts := tftp.Server{
		Handler: handleTFTP,
		InfoLog: func(msg string) { log.Logger.Info().Str("subsystem", "TFTP").Msg(msg) },
		TransferLog: func(clientAddr net.Addr, path string, err error) {
			log.Logger.Debug().Str("subsystem", "TFTP").Str("path", path).Err(err).Str("client", clientAddr.String()).Msgf("Transfer")
		},
	}
	err := ts.Serve(conn)
	if err != nil {
		return fmt.Errorf("TFTP server shut down: %s", err)
	}
	return nil
}

// handleTFTP handles TFTP requests. It serves the EFI key enroller file
// and does not process other types of requests.
func handleTFTP(_ string, _ net.Addr) (io.ReadCloser, int64, error) {
	return io.NopCloser(bytes.NewBuffer(constants.EfiKeyEnroller)), int64(len(constants.EfiKeyEnroller)), nil
}

func serveHTTP(keydir string, isoFile string, log types.KairosLogger) error {
	// Build a map of lowercased filenames to actual filenames in ./keys
	filesMap := make(map[string]string)
	entries, err := os.ReadDir(keydir)
	if err != nil {
		log.Logger.Error().Err(err).Msg("Failed to read keys directory")
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			filesMap[strings.ToLower(entry.Name())] = entry.Name()
		}
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// we serve the full iso so its simpler
		if r.URL.Path == "/kairos.iso" {
			log.Logger.Info().Str("method", r.Method).Str("url", r.URL.Path).Msg("Serving kairos.iso from root")
			http.ServeFile(w, r, isoFile)
			return
		}
		// This serves the booter.efi directly, which is key enroller
		if r.URL.Path == "/booter.efi" {
			log.Logger.Info().Str("method", r.Method).Str("url", r.URL.Path).Msg("Serving booter.efi from memory")
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", "attachment; filename=\"booter.efi\"")
			w.Write(constants.EfiKeyEnroller) // assuming you have a []byte constant
			return
		}
		// Case-insensitive lookup for files in ./keys
		requested := strings.TrimPrefix(r.URL.Path, "/")
		actual, ok := filesMap[strings.ToLower(requested)]
		if ok {
			filePath := filepath.Join(keydir, actual)
			log.Logger.Info().Str("method", r.Method).Str("url", r.URL.Path).Str("file", filePath).Msg("Serving file from keys dir (case-insensitive)")
			http.ServeFile(w, r, filePath)
			return
		}
		log.Logger.Info().Str("url", r.URL.Path).Msg("File not found (case-insensitive)")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("404 not found"))
	})

	http.Handle("/", handler)
	listenAddr := os.Getenv("HTTP_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":80"
	}
	log.Logger.Info().Str("subsystem", "HTTP").Msgf("Listening for requests on %s", listenAddr)
	err = http.ListenAndServe(listenAddr, nil)
	if err != nil {
		log.Logger.Error().Err(err).Msg("Error starting HTTP server")
	}
	return err
}
