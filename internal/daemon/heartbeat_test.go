package deamon

import (
	"fmt"
	"testing"
	"time"
)

func TestBeat(t *testing.T) {
	message := []byte{Beat}
	sendUDPMessage(message, "127.0.0.1:40001")
}

func TestServer(t *testing.T) {
	serverArr := make([]HeartbeatServer, 0)
	member := make([]string, 0)
	for i := range 5 {
		host := fmt.Sprintf("127.0.0.1:%d", 40000+i)
		member = append(member, host)
	}

	for _, host := range member {
		server := new(HeartbeatServer)
		server.init()
		server.host = host
		server.member = make([]string, len(member))
		copy(server.member, member)
		serverArr = append(serverArr, *server)
	}

	for _, server := range serverArr {
		go server.start()
	}

	for {
		time.Sleep(1 * time.Second)
		time.Sleep(1 * time.Second)
	}
}
