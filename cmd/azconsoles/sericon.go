package main

import (
	"log"
	"os"

	"github.com/gobwas/ws/wsutil"

	"github.com/brendank310/azconsoles/pkg/azconsoles"
)

func main() {
	subscriptionId := os.Getenv("SUBSCRIPTION_ID")
	resourceGroup := os.Getenv("RESOURCE_GROUP")
	vmName := os.Getenv("VM_NAME")
	conn, err := azconsoles.StartSerialConsole(subscriptionId, resourceGroup, vmName)
	if err != nil {
		panic(err)
	}

	for {
		rxBuf, err := wsutil.ReadServerText(conn)
		if err != nil {
			return
		}

		//wsutil.WriteClientText(conn, []byte("somestuff"))
		log.Printf("%v", string(rxBuf))
	}
}
