package netbootmgr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Status represents the current state of the netboot server.
type Status struct {
	Running    bool   `json:"running"`
	ArtifactID string `json:"artifactId,omitempty"`
	Address    string `json:"address"`
	Port       string `json:"port"`
}

// Manager manages a PXE/netboot server lifecycle.
type Manager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	status  Status
	address string
	port    string
}

// NewManager creates a new netboot Manager with default settings.
func NewManager() *Manager {
	return &Manager{
		address: "0.0.0.0",
		port:    "8090",
	}
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
		Port:       m.port,
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
