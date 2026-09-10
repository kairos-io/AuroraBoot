package netbootmgr

import (
	"bytes"
	"fmt"
	"io"
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

// LogSink receives the netboot server's output as it happens, so a UI client
// watching a PXE boot in progress can see why it fails instead of only a
// stalled "Running" badge (kairos-io/kairos#4596). Satisfied by
// *ws.UIHub.BroadcastNetbootLogChunk; kept as a narrow interface here so this
// package does not import pkg/ws.
type LogSink interface {
	BroadcastNetbootLogChunk(chunk string)
}

// maxLogBytes bounds the retained log so a long-running or noisy PXE server
// (a node retrying DHCP forever) cannot grow this without limit; it is an
// in-memory debugging aid, not a durable log store.
const maxLogBytes = 256 * 1024

// Manager manages a PXE/netboot server lifecycle.
type Manager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	status  Status
	address string
	port    string
	logSink LogSink
	logBuf  bytes.Buffer
}

// NewManager creates a new netboot Manager with default settings. logSink may
// be nil (e.g. in tests), in which case output is still captured for
// GetLogs but never broadcast live.
func NewManager(logSink LogSink) *Manager {
	return &Manager{
		address: "0.0.0.0",
		port:    "8090",
		logSink: logSink,
	}
}

// logWriter tees a running command's output to the process's own
// stdout/stderr (unchanged operational behaviour), into the Manager's bounded
// snapshot buffer, and — line by line — to the live broadcaster.
type logWriter struct {
	m      *Manager
	passOn io.Writer
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.m.mu.Lock()
	if w.m.logBuf.Len()+len(p) > maxLogBytes {
		// Drop the oldest bytes rather than the newest: a debugging session
		// cares about what just happened, not the start of a long-idle server.
		overflow := w.m.logBuf.Len() + len(p) - maxLogBytes
		b := w.m.logBuf.Bytes()
		w.m.logBuf.Reset()
		if overflow < len(b) {
			w.m.logBuf.Write(b[overflow:])
		}
	}
	w.m.logBuf.Write(p)
	sink := w.m.logSink
	w.m.mu.Unlock()

	if sink != nil {
		sink.BroadcastNetbootLogChunk(string(p))
	}
	return w.passOn.Write(p)
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
	// A fresh session starts with a clean log: the previous run's output (if
	// any) is no longer relevant to debugging this one.
	m.logBuf.Reset()
	cmd.Stdout = &logWriter{m: m, passOn: os.Stdout}
	cmd.Stderr = &logWriter{m: m, passOn: os.Stderr}

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

// GetLogs returns a snapshot of the current (or most recently run) netboot
// session's captured stdout/stderr, bounded to maxLogBytes. Empty before the
// first Start call.
func (m *Manager) GetLogs() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.logBuf.String()
}
