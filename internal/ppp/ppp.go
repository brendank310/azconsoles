package ppp

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	}
}

// Manager manages a PPP connection
type Manager struct {
	config     Config
	cmd        *exec.Cmd
	symlinkPath string
}

// NewManager creates a new PPP manager
func NewManager(config Config) *Manager {
	return &Manager{
		config: config,
	}
}

// Start sets up PPP over the given file descriptor
func (m *Manager) Start(ctx context.Context, fd *os.File) error {
	// Create symlink to the file descriptor
	symlinkPath, err := m.createSymlink(fd)
	if err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}
	m.symlinkPath = symlinkPath

	// Start pppd
	args := []string{
		m.config.PppdPath,
		symlinkPath,
		strconv.Itoa(m.config.Baud),
		"local",
		"noauth",
		"nodetach",
		"ipcp-accept-local",
		"ipcp-accept-remote",
		fmt.Sprintf("%s:%s", m.config.LocalIP, m.config.PeerIP),
		fmt.Sprintf("mtu %d", m.config.MTU),
		fmt.Sprintf("mru %d", m.config.MRU),
		"nocrtscts", // Default to no modem control
	}

	m.cmd = exec.CommandContext(ctx, args[0], args[1:]...)
	m.cmd.Stderr = os.Stderr // Let pppd errors go to stderr

	if err := m.cmd.Start(); err != nil {
		m.cleanup()
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
	var err error
	
	// Terminate pppd if running
	if m.cmd != nil && m.cmd.Process != nil {
		if killErr := m.cmd.Process.Signal(syscall.SIGTERM); killErr != nil {
			// Force kill if SIGTERM doesn't work
			m.cmd.Process.Kill()
		}
		m.cmd.Wait() // Wait for process to exit
	}

	// Clean up symlink
	if cleanupErr := m.cleanup(); cleanupErr != nil {
		err = cleanupErr
	}

	return err
}

// GetPeerIP returns the peer IP address
func (m *Manager) GetPeerIP() string {
	return m.config.PeerIP
}

// createSymlink creates a symlink to the file descriptor
func (m *Manager) createSymlink(fd *os.File) (string, error) {
	// Create unique symlink path
	pid := os.Getpid()
	fdNum := fd.Fd()
	symlinkPath := filepath.Join("/tmp", fmt.Sprintf("sericonssh-tty-%d-%d", pid, fdNum))
	
	// Target path in /proc
	fdPath := fmt.Sprintf("/proc/%d/fd/%d", pid, fdNum)
	
	// Remove existing symlink if it exists
	os.Remove(symlinkPath)
	
	// Create symlink
	if err := os.Symlink(fdPath, symlinkPath); err != nil {
		return "", fmt.Errorf("failed to create symlink %s -> %s: %w", symlinkPath, fdPath, err)
	}
	
	return symlinkPath, nil
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

// cleanup removes the symlink
func (m *Manager) cleanup() error {
	if m.symlinkPath != "" {
		err := os.Remove(m.symlinkPath)
		m.symlinkPath = ""
		return err
	}
	return nil
}

// CheckPppdAvailable checks if pppd is available and executable
func CheckPppdAvailable(pppdPath string) error {
	if pppdPath == "" {
		pppdPath = "/usr/sbin/pppd"
	}

	// Check if file exists and is executable
	if _, err := os.Stat(pppdPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("pppd not found at %s. Install ppp package or specify correct path with --pppd", pppdPath)
		}
		return fmt.Errorf("cannot access pppd at %s: %w", pppdPath, err)
	}

	// Try to execute pppd --help to verify it works
	cmd := exec.Command(pppdPath, "--help")
	if err := cmd.Run(); err != nil {
		// Check if it's a permission issue
		if strings.Contains(err.Error(), "permission denied") {
			return fmt.Errorf("insufficient permissions to run pppd. Try running with sudo or ensure pppd has appropriate permissions")
		}
		return fmt.Errorf("pppd at %s is not working properly: %w", pppdPath, err)
	}

	return nil
}