package main

import (
	"log"

	//"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"

	"github.com/brendank310/azconsoles/pkg/azconsoles"
)

func main() {
	conn, err := azconsoles.ConnectCloudShell()
	if err != nil {
		panic(err)
	}

	for {
		rxBuf, err := wsutil.ReadServerText(conn)
		if err != nil {
			return
		}

		//wsutil.WriteClientText(conn, []byte(""))
		log.Printf("%v", string(rxBuf))
	}
}
