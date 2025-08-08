package ppp

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Config represents PPP configuration
type Config struct {
	LocalIP    string        // Local IP address (e.g., "10.7.0.2")
	PeerIP     string        // Peer IP address (e.g., "10.7.0.1")
	Baud       int           // Baud rate
	MTU        int           // MTU size
	MRU        int           // MRU size
	Timeout    time.Duration // Timeout for PPP to come up
	PppdPath   string        // Path to pppd binary
	KeepOnExit bool          // Keep PPP interface on exit (debug mode)
	PtyCommand string        // Command to run as PTY for pppd
}

// DefaultConfig returns default PPP configuration
func DefaultConfig() Config {
	return Config{
		LocalIP:    "10.7.0.2",
		PeerIP:     "10.7.0.1",
		Baud:       115200,
		MTU:        1500,
		MRU:        1500,
		Timeout:    30 * time.Second,
		PppdPath:   "/usr/sbin/pppd",
		KeepOnExit: false,
		PtyCommand: "",
	}
}

// Manager manages a PPP connection
type Manager struct {
	config Config
	cmd    *exec.Cmd
}

// NewManager creates a new PPP manager
func NewManager(config Config) *Manager {
	return &Manager{
		config: config,
	}
}

// Start sets up PPP using pppd's native pty option
func (m *Manager) Start(ctx context.Context) error {
	if m.config.PtyCommand == "" {
		return fmt.Errorf("PtyCommand is required")
	}

	// Build pppd arguments using native pty option
	args := []string{
		"updetach",
		"noauth",
		"local",
		"nocrtscts",
		"ipcp-accept-local",
		"ipcp-accept-remote",
		fmt.Sprintf("%s:%s", m.config.LocalIP, m.config.PeerIP),
		"mtu", fmt.Sprintf("%d", m.config.MTU),
		"mru", fmt.Sprintf("%d", m.config.MRU),
		"connect-delay", "1000",
		"child-timeout", "10",
		"pty", m.config.PtyCommand,
	}

	m.cmd = exec.CommandContext(ctx, m.config.PppdPath, args...)
	m.cmd.Stdout = os.Stderr // Let pppd output go to stderr
	m.cmd.Stderr = os.Stderr

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start pppd: %w", err)
	}

	// Wait for PPP interface to come up
	if err := m.waitForInterface(ctx); err != nil {
		m.Stop()
		return fmt.Errorf("PPP interface failed to come up: %w", err)
	}

	return nil
}

// Stop terminates the PPP connection
func (m *Manager) Stop() error {
	// Terminate pppd if running
	if m.cmd != nil && m.cmd.Process != nil {
		if killErr := m.cmd.Process.Signal(syscall.SIGTERM); killErr != nil {
			// Force kill if SIGTERM doesn't work
			m.cmd.Process.Kill()
		}
		m.cmd.Wait() // Wait for process to exit
	}

	return nil
}

// GetPeerIP returns the peer IP address
func (m *Manager) GetPeerIP() string {
	return m.config.PeerIP
}

// waitForInterface waits for the ppp0 interface to come up with the expected IP
func (m *Manager) waitForInterface(ctx context.Context) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(m.config.Timeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for PPP interface after %v", m.config.Timeout)
		case <-ticker.C:
			if m.checkInterface() {
				return nil
			}
		}
	}
}

// checkInterface checks if ppp0 has the expected local IP
func (m *Manager) checkInterface() bool {
	iface, err := net.InterfaceByName("ppp0")
	if err != nil {
		return false
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.String() == m.config.LocalIP {
				return true
			}
		}
	}

	return false
}

// CheckPppdAvailable checks if pppd is available and executable
func CheckPppdAvailable(pppdPath string) error {
	if pppdPath == "" {
		pppdPath = "/usr/sbin/pppd"
	}

	// Check if file exists and is executable
	fileInfo, err := os.Stat(pppdPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("pppd not found at %s. Install ppp package or specify correct path with --pppd", pppdPath)
		}
		return fmt.Errorf("cannot access pppd at %s: %w", pppdPath, err)
	}

	// Check if we can execute pppd - it requires either setuid root or dip group membership
	cmd := exec.Command(pppdPath, "--help")
	if err := cmd.Run(); err != nil {
		// Check if it's a permission issue
		if strings.Contains(err.Error(), "permission denied") {
			// Provide detailed guidance based on file permissions
			mode := fileInfo.Mode()
			var suggestion string
			if mode&os.ModeSetuid != 0 {
				// pppd is setuid, might need to be in dip group
				suggestion = "Run with sudo, add user to 'dip' group (sudo usermod -a -G dip $USER), or ensure pppd has setuid permissions"
			} else {
				suggestion = "Run with sudo or ensure pppd has setuid root permissions"
			}
			return fmt.Errorf("insufficient permissions to run pppd. %s", suggestion)
		}
		return fmt.Errorf("pppd at %s is not working properly: %w", pppdPath, err)
	}

	return nil
}
