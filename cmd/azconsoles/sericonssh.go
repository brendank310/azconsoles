package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gobwas/ws/wsutil"
	"github.com/brendank310/azconsoles/internal/ppp"
	"github.com/brendank310/azconsoles/pkg/sericon"
)

// Target represents the parsed target specification
type Target struct {
	User          string
	VMName        string
	ResourceGroup string
	Subscription  string
}

// Config represents the CLI configuration
type Config struct {
	Target     Target
	Baud       int
	PPPLocal   string
	PPPPeer    string
	SSHPort    int
	Timeout    time.Duration
	PppdPath   string
	LogLevel   string
	KeepPPP    bool
	DryRun     bool
	SSHArgs    []string
}

func main() {
	config, err := parseArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if config.LogLevel == "debug" {
		log.Printf("Config: %+v", config)
	}

	if config.DryRun {
		printPlan(config)
		return
	}

	// Check pppd availability early
	if err := ppp.CheckPppdAvailable(config.PppdPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := run(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func parseArgs() (*Config, error) {
	config := &Config{
		Baud:     115200,
		PPPLocal: "10.7.0.2",
		PPPPeer:  "10.7.0.1",
		SSHPort:  2222,
		Timeout:  30 * time.Second,
		PppdPath: "/usr/sbin/pppd",
		LogLevel: "info",
	}

	flag.IntVar(&config.Baud, "baud", config.Baud, "Serial baud rate")
	flag.StringVar(&config.PPPLocal, "ppp-local", config.PPPLocal, "Local PPP IP address")
	flag.StringVar(&config.PPPPeer, "ppp-peer", config.PPPPeer, "Peer PPP IP address")
	flag.IntVar(&config.SSHPort, "ssh-port", config.SSHPort, "SSH port on PPP peer")
	flag.DurationVar(&config.Timeout, "timeout", config.Timeout, "PPP up wait timeout")
	flag.StringVar(&config.PppdPath, "pppd", config.PppdPath, "Path to pppd binary")
	flag.StringVar(&config.LogLevel, "log-level", config.LogLevel, "Log level (info|debug)")
	flag.BoolVar(&config.KeepPPP, "keep-ppp", config.KeepPPP, "Do not kill PPP on exit (debug)")
	flag.BoolVar(&config.DryRun, "dry-run", config.DryRun, "Print plan without executing")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] <user>@<vm>.<rg>.<sub> [-- <ssh-args>]\n\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "SSH over Azure Serial Console using PPP\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\nExamples:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s user@myvm.myrg.12345678-1234-1234-1234-123456789012\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "  %s user@myvm.myrg.sub -- -v -o StrictHostKeyChecking=no\n", os.Args[0])
	}

	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		return nil, fmt.Errorf("missing target specification")
	}

	// Parse target
	target, err := parseTarget(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid target: %w", err)
	}
	config.Target = target

	// Handle -- separator for SSH args
	sshArgs := []string{}
	for i, arg := range args[1:] {
		if arg == "--" {
			sshArgs = args[i+2:] // Everything after --
			break
		}
		sshArgs = append(sshArgs, arg)
	}
	config.SSHArgs = sshArgs

	// Validate log level
	if config.LogLevel != "info" && config.LogLevel != "debug" {
		return nil, fmt.Errorf("invalid log level: %s (must be info or debug)", config.LogLevel)
	}

	return config, nil
}

func parseTarget(target string) (Target, error) {
	// Format: user@vm.rg.sub
	// Where sub can be a subscription ID or shorthand

	parts := strings.Split(target, "@")
	if len(parts) != 2 {
		return Target{}, fmt.Errorf("target must be in format user@vm.rg.sub")
	}

	user := parts[0]
	if user == "" {
		return Target{}, fmt.Errorf("user cannot be empty")
	}

	hostParts := strings.Split(parts[1], ".")
	if len(hostParts) != 3 {
		return Target{}, fmt.Errorf("host part must be in format vm.rg.sub")
	}

	vmName, resourceGroup, subscription := hostParts[0], hostParts[1], hostParts[2]

	// Validate components contain only allowed characters
	nameRegex := regexp.MustCompile(`^[a-zA-Z0-9\-_\.]+$`)
	if !nameRegex.MatchString(vmName) {
		return Target{}, fmt.Errorf("VM name contains invalid characters")
	}
	if !nameRegex.MatchString(resourceGroup) {
		return Target{}, fmt.Errorf("resource group contains invalid characters")
	}

	// Validate subscription (UUID format or allowed chars)
	uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	if !uuidRegex.MatchString(subscription) && !nameRegex.MatchString(subscription) {
		return Target{}, fmt.Errorf("subscription ID format is invalid")
	}

	// If subscription is not a UUID, try to get from environment
	if !uuidRegex.MatchString(subscription) {
		envSub := os.Getenv("AZURE_SUBSCRIPTION_ID")
		if envSub == "" {
			return Target{}, fmt.Errorf("subscription '%s' is not a UUID and AZURE_SUBSCRIPTION_ID environment variable is not set", subscription)
		}
		subscription = envSub
	}

	return Target{
		User:          user,
		VMName:        vmName,
		ResourceGroup: resourceGroup,
		Subscription:  subscription,
	}, nil
}

func printPlan(config *Config) {
	fmt.Printf("Execution Plan:\n")
	fmt.Printf("1. Connect to Azure VM serial console:\n")
	fmt.Printf("   - Subscription: %s\n", config.Target.Subscription)
	fmt.Printf("   - Resource Group: %s\n", config.Target.ResourceGroup)
	fmt.Printf("   - VM: %s\n", config.Target.VMName)
	fmt.Printf("2. Establish PPP connection:\n")
	fmt.Printf("   - Local IP: %s\n", config.PPPLocal)
	fmt.Printf("   - Peer IP: %s\n", config.PPPPeer)
	fmt.Printf("   - Baud: %d\n", config.Baud)
	fmt.Printf("   - Timeout: %v\n", config.Timeout)
	fmt.Printf("3. Execute SSH:\n")
	fmt.Printf("   - Target: %s@%s:%d\n", config.Target.User, config.PPPPeer, config.SSHPort)
	if len(config.SSHArgs) > 0 {
		fmt.Printf("   - Args: %v\n", config.SSHArgs)
	}
}

func run(config *Config) error {
	// Set up signal handling for graceful cleanup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Connect to Azure serial console
	if config.LogLevel == "debug" {
		log.Printf("Connecting to Azure serial console...")
	}

	serialConfig := sericon.Config{
		SubscriptionID: config.Target.Subscription,
		ResourceGroup:  config.Target.ResourceGroup,
		VMName:         config.Target.VMName,
	}

	serialConn, err := sericon.Connect(serialConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to serial console: %w", err)
	}
	defer serialConn.Close()

	if config.LogLevel == "debug" {
		log.Printf("Serial connection established")
	}

	// Set up PPP
	pppConfig := ppp.Config{
		LocalIP:    config.PPPLocal,
		PeerIP:     config.PPPPeer,
		Baud:       config.Baud,
		MTU:        1500,
		MRU:        1500,
		Timeout:    config.Timeout,
		PppdPath:   config.PppdPath,
		KeepOnExit: config.KeepPPP,
	}

	pppMgr := ppp.NewManager(pppConfig)

	// Create a pipe for serial data forwarding
	serialFd := createSerialForwarder(ctx, serialConn)
	defer serialFd.Close()

	// Start PPP
	if config.LogLevel == "debug" {
		log.Printf("Starting PPP...")
	}

	if err := pppMgr.Start(ctx, serialFd); err != nil {
		return fmt.Errorf("failed to start PPP: %w", err)
	}

	if !config.KeepPPP {
		defer func() {
			if config.LogLevel == "debug" {
				log.Printf("Stopping PPP...")
			}
			pppMgr.Stop()
		}()
	}

	if config.LogLevel == "info" || config.LogLevel == "debug" {
		log.Printf("PPP connection established to %s", config.PPPPeer)
	}

	// Handle cleanup on signals
	go func() {
		<-sigChan
		if config.LogLevel == "debug" {
			log.Printf("Received signal, cleaning up...")
		}
		cancel()
	}()

	// Execute SSH
	sshArgs := []string{"ssh"}
	sshArgs = append(sshArgs, fmt.Sprintf("%s@%s", config.Target.User, config.PPPPeer))
	sshArgs = append(sshArgs, "-p", strconv.Itoa(config.SSHPort))
	sshArgs = append(sshArgs, config.SSHArgs...)

	if config.LogLevel == "debug" {
		log.Printf("Executing: %v", sshArgs)
	}

	sshCmd := exec.CommandContext(ctx, "ssh", sshArgs[1:]...)
	sshCmd.Stdin = os.Stdin
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr

	return sshCmd.Run()
}

// createSerialForwarder creates a pipe that forwards data between
// the serial connection and a file descriptor that pppd can use
func createSerialForwarder(ctx context.Context, serialConn *sericon.Connection) *os.File {
	r, w, err := os.Pipe()
	if err != nil {
		log.Fatalf("Failed to create pipe: %v", err)
	}

	rawConn := serialConn.GetRawConnection()

	// Forward from serial to pipe (for pppd to read)
	go func() {
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				data, err := wsutil.ReadServerBinary(rawConn)
				if err != nil {
					if ctx.Err() == nil {
						log.Printf("Error reading from serial: %v", err)
					}
					return
				}
				if _, err := w.Write(data); err != nil {
					if ctx.Err() == nil {
						log.Printf("Error writing to pipe: %v", err)
					}
					return
				}
			}
		}
	}()

	// Forward from pipe to serial (for pppd to write)
	go func() {
		defer r.Close()
		buf := make([]byte, 1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := r.Read(buf)
				if err != nil {
					if err != io.EOF && ctx.Err() == nil {
						log.Printf("Error reading from pipe: %v", err)
					}
					return
				}
				if err := wsutil.WriteClientBinary(rawConn, buf[:n]); err != nil {
					if ctx.Err() == nil {
						log.Printf("Error writing to serial: %v", err)
					}
					return
				}
			}
		}
	}()

	return r
}