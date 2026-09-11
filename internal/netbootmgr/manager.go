package netbootmgr

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// managerContextKey is unexported so only this package can set or read the
// value it names -- callers use WithManager/FromContext instead.
type managerContextKey struct{}

// WithManager returns a copy of ctx carrying m, retrievable with FromContext.
// The deployer's netboot step (deployer/steps.go's StepStartNetboot) uses
// this to route an artifact-build-triggered netboot start through the same
// Manager the dashboard's Start/Stop/Status endpoints use, instead of
// starting a server the UI has no way to see or stop -- see
// mission-control's incident notes on the two code paths not sharing state.
func WithManager(ctx context.Context, m *Manager) context.Context {
	return context.WithValue(ctx, managerContextKey{}, m)
}

// FromContext returns the Manager stored by WithManager, or nil if none was
// set -- the normal case for the CLI, which has no dashboard to keep in sync.
func FromContext(ctx context.Context) *Manager {
	m, _ := ctx.Value(managerContextKey{}).(*Manager)
	return m
}

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

	// If the artifact was built with a cloud-config attached, it's saved
	// alongside it as config.yaml. Serve it as config_url so kairos-agent's
	// sdk/collector (which reads config_url from /proc/cmdline) can fetch
	// it at boot -- the /oem datasource path only pulls from cloud-provider
	// metadata services, never from netboot's own HTTP delivery, so this is
	// the only way a netbooted bare-metal node gets configured at all.
	cloudConfig := ""
	if cfgPath := filepath.Join(artifactsDir, artifactID, "config.yaml"); fileExists(cfgPath) {
		cloudConfig = cfgPath
	}

	return m.StartWithPaths(artifactID, cloudConfig, squashfs, initrd, kernel)
}

// StartWithPaths launches the netboot server from already-resolved file
// paths, bypassing the <artifactsDir>/<artifactID>/netboot/ convention Start
// assumes. artifactID is used only for Status.ArtifactID; it need not be a
// real artifact store ID. Exported so a caller with its own paths in hand
// (deployer/steps.go's StepStartNetboot, via WithManager/FromContext) can
// still register with this Manager's shared state -- see WithManager's doc.
func (m *Manager) StartWithPaths(artifactID, cloudConfig, squashfs, initrd, kernel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.status.Running {
		return fmt.Errorf("netboot server is already running")
	}

	// AuroraBoot start-pixie args: <cloud-config> <squashfs> <address> <port> <initrd> <kernel>
	// cloud-config may be empty if the artifact was built with none attached.
	cmd := exec.Command("auroraboot", "start-pixie", cloudConfig, squashfs, m.address, m.port, initrd, kernel)
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
