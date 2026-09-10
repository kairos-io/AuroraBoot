package netbootmgr

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// bindAddress is the address the netboot server listens on. It is a wildcard
// because the server has to answer PXE clients on whichever interface they
// arrive from, which is also why it can never double as the address to hand
// out; see AdvertisedAddress.
const bindAddress = "0.0.0.0"

// Status represents the current state of the netboot server.
type Status struct {
	Running    bool   `json:"running"`
	ArtifactID string `json:"artifactId,omitempty"`
	// Address is the address the server binds to, always the wildcard.
	Address string `json:"address"`
	// AdvertisedAddress is the host to reach the server on: the host of the
	// configured external URL, or a local interface address when there is none.
	AdvertisedAddress string `json:"advertisedAddress"`
	Port              string `json:"port"`
}

// Manager manages a PXE/netboot server lifecycle.
type Manager struct {
	mu            sync.Mutex
	cmd           *exec.Cmd
	status        Status
	address       string
	advertisedURL string
	port          string
}

// NewManager creates a new netboot Manager. advertisedURL is the externally
// reachable URL of this AuroraBoot instance (--url / AURORABOOT_URL); only its
// host is used, since the netboot server has its own port. It may be empty.
func NewManager(advertisedURL string) *Manager {
	return &Manager{
		address:       bindAddress,
		advertisedURL: advertisedURL,
		port:          "8090",
	}
}

// advertisedHost is the host clients should use to reach the netboot server.
// The configured external URL wins; failing that a local interface address is
// better than handing out the wildcard, which is reachable from nowhere.
func advertisedHost(advertisedURL string) string {
	if h := hostFromURL(advertisedURL); h != "" {
		return h
	}
	if ip := localIPv4(); ip != "" {
		return ip
	}
	return bindAddress
}

// hostFromURL extracts the host from a URL, tolerating a bare "host" or
// "host:port" with no scheme, which url.Parse reads as a scheme or a path.
func hostFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Once a scheme separator is present url.Parse is authoritative. Falling
	// through to SplitHostPort would read "http://" as host "http", port "//".
	if strings.Contains(raw, "//") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		return u.Hostname()
	}
	if h, _, err := net.SplitHostPort(raw); err == nil && h != "" {
		return h
	}
	if !strings.ContainsAny(raw, "/:") {
		return raw
	}
	return ""
}

// localIPv4 returns an IPv4 address of the first up, non-loopback interface.
// Link-local addresses are skipped: they name an interface nobody can route to.
func localIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil || ip4.IsLinkLocalUnicast() {
				continue
			}
			return ip4.String()
		}
	}
	return ""
}

// Start launches the netboot server for the given artifact.
// It looks for kairos-kernel, kairos-initrd, and kairos.squashfs
// in <artifactsDir>/<artifactID>/netboot/.
func (m *Manager) Start(artifactsDir, artifactID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.status.Running {
		return fmt.Errorf("netboot server is already running")
	}

	netbootDir := filepath.Join(artifactsDir, artifactID, "netboot")

	kernel := filepath.Join(netbootDir, "kairos-kernel")
	initrd := filepath.Join(netbootDir, "kairos-initrd")
	squashfs := filepath.Join(netbootDir, "kairos.squashfs")

	// Verify required files exist.
	for _, f := range []string{kernel, initrd, squashfs} {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("required netboot file not found: %s", f)
		}
	}

	// AuroraBoot start-pixie args: <cloud-config> <squashfs> <address> <port> <initrd> <kernel>
	// Use empty string for cloud-config (not required for netboot).
	cmd := exec.Command("auroraboot", "start-pixie", "", squashfs, m.address, m.port, initrd, kernel)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start netboot server: %w", err)
	}

	m.cmd = cmd
	m.status = Status{
		Running:    true,
		ArtifactID: artifactID,
		Address:    m.address,
		// Resolved per start, not once in NewManager, so an interface that came
		// up after the fleet server did is still picked up.
		AdvertisedAddress: advertisedHost(m.advertisedURL),
		Port:              m.port,
	}

	// Wait for the process in the background so we can detect if it exits.
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		// Only clear status if this is still the active command.
		if m.cmd == cmd {
			m.status.Running = false
			m.cmd = nil
		}
	}()

	return nil
}

// Stop kills the running netboot server subprocess.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.status.Running || m.cmd == nil {
		return fmt.Errorf("netboot server is not running")
	}

	if m.cmd.Process != nil {
		if err := m.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to stop netboot server: %w", err)
		}
	}

	m.cmd = nil
	m.status.Running = false
	m.status.ArtifactID = ""

	return nil
}

// GetStatus returns the current netboot server status.
func (m *Manager) GetStatus() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}
