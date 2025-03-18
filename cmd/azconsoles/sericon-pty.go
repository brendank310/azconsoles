package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"github.com/gobwas/ws/wsutil"
	"github.com/brendank310/azconsoles/pkg/azconsoles"
)

func main() {
	// Get configuration from environment variables.
	subscriptionId := os.Getenv("SUBSCRIPTION_ID")
	resourceGroup := os.Getenv("RESOURCE_GROUP")
	vmName := os.Getenv("VM_NAME")

	// Start the websocket connection to the Azure VM's serial console.
	conn, err := azconsoles.StartSerialConsole(subscriptionId, resourceGroup, vmName)
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

