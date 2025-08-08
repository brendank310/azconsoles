package sericon

import (
	"net"
	"os"

	"github.com/brendank310/azconsoles/pkg/azconsoles"
)

// Config represents the configuration for connecting to Azure serial console
type Config struct {
	SubscriptionID string
	ResourceGroup  string
	VMName         string
}

// Connection represents a serial console connection
type Connection struct {
	conn net.Conn
}

// Connect establishes a connection to Azure VM serial console
func Connect(config Config) (*Connection, error) {
	conn, err := azconsoles.StartSerialConsole(
		config.SubscriptionID,
		config.ResourceGroup,
		config.VMName,
	)
	if err != nil {
		return nil, err
	}

	return &Connection{conn: conn}, nil
}

// GetRawConnection returns the underlying net.Conn for direct access
func (c *Connection) GetRawConnection() net.Conn {
	return c.conn
}

// Close closes the serial console connection
func (c *Connection) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ConnectFromEnv creates a connection using environment variables
func ConnectFromEnv() (*Connection, error) {
	config := Config{
		SubscriptionID: os.Getenv("SUBSCRIPTION_ID"),
		ResourceGroup:  os.Getenv("RESOURCE_GROUP"),
		VMName:         os.Getenv("VM_NAME"),
	}
	return Connect(config)
}