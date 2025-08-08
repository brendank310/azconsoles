package main

import (
	"flag"
	"fmt"
	"github.com/brendank310/azconsoles/pkg/azconsoles"
	"github.com/creack/pty"
	"github.com/gobwas/ws/wsutil"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func main() {
	// Command line arguments
	var (
		subscriptionId = flag.String("subscription", "", "Azure subscription ID")
		resourceGroup  = flag.String("resource-group", "", "Azure resource group")
		vmName         = flag.String("vm-name", "", "Azure VM name")
		speed          = flag.Int("speed", 115200, "Baud rate (ignored - for compatibility)")
		help           = flag.Bool("help", false, "Show help")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Connect to Azure VM serial console via PTY\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables (fallback):\n")
		fmt.Fprintf(os.Stderr, "  SUBSCRIPTION_ID  - Azure subscription ID\n")
		fmt.Fprintf(os.Stderr, "  RESOURCE_GROUP   - Azure resource group\n")
		fmt.Fprintf(os.Stderr, "  VM_NAME          - Azure VM name\n")
	}

	flag.Parse()

	if *help {
		flag.Usage()
		return
	}

	// Get configuration from command line or environment variables
	subId := *subscriptionId
	if subId == "" {
		subId = os.Getenv("SUBSCRIPTION_ID")
	}

	rgName := *resourceGroup
	if rgName == "" {
		rgName = os.Getenv("RESOURCE_GROUP")
	}

	vmNameValue := *vmName
	if vmNameValue == "" {
		vmNameValue = os.Getenv("VM_NAME")
	}

	// Validate required parameters
	if subId == "" {
		log.Fatalf("subscription ID is required (use --subscription or SUBSCRIPTION_ID env var)")
	}
	if rgName == "" {
		log.Fatalf("resource group is required (use --resource-group or RESOURCE_GROUP env var)")
	}
	if vmNameValue == "" {
		log.Fatalf("VM name is required (use --vm-name or VM_NAME env var)")
	}

	// Log baud rate for debugging (not actually used by Azure serial console)
	if *speed != 115200 {
		log.Printf("Note: baud rate %d specified but Azure serial console uses websocket transport", *speed)
	}

	// Start the websocket connection to the Azure VM's serial console.
	conn, err := azconsoles.StartSerialConsole(subId, rgName, vmNameValue)
	if err != nil {
		log.Fatalf("failed to start serial console: %v", err)
	}
	defer conn.Close()

	// Create a pseudo-terminal.
	ptmx, tty, err := pty.Open()
	if err != nil {
		log.Fatalf("failed to open pty: %v", err)
	}
	defer ptmx.Close()

	// Print the name of the slave device for use with slattach.
	fmt.Printf("Pseudo-tty slave device: %s\n", tty.Name())

	// Set up signal handler for graceful exit.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		ptmx.Close()
		os.Exit(0)
	}()

	// Set the PTY to raw mode
	oldState, err := term.MakeRaw(int(ptmx.Fd()))
	if err != nil {
		log.Fatal(err)
	}
	defer term.Restore(int(ptmx.Fd()), oldState)

	// Start a goroutine to forward data from the websocket to the pty.
	go func() {
		for {
			// Read a binary message from the websocket.
			data, err := wsutil.ReadServerBinary(conn)
			if err != nil {
				log.Printf("error reading from websocket: %v", err)
				return
			}
			// Write the received data to the pty.
			_, err = ptmx.Write(data)
			if err != nil {
				log.Printf("error writing to pty: %v", err)
				return
			}
		}
	}()

	// Start a goroutine to forward data from the pty to the websocket.
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("error reading from pty: %v", err)
				}
				return
			}
			// Write the read data as a binary message to the websocket.
			err = wsutil.WriteClientBinary(conn, buf[:n])
			if err != nil {
				log.Printf("error writing to websocket: %v", err)
				return
			}
		}
	}()

	// Block forever.
	select {}
}
