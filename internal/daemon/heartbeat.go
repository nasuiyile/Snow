package deamon

import (
	"fmt"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
)

type EventType = byte

const (
	// 心跳检测
	Beat EventType = iota
	// 转发心跳
	RelayBeat
	// 回复心跳
	ReplyBeat
)

type HeartState = byte

const (
	Polling HeartState = iota
	Transfer
)

type HeartbeatServer struct {
	host       string
	udpTimeout int
	conn       *net.UDPConn
	member     []string
}

func (server *HeartbeatServer) init() {
	server.udpTimeout = 10
	// server.host = fmt.Sprintf("127.0.0.1:%d", port)
	// server.member = make([]string, 0)
	// server.member[server.host] = Start
	// server.member["127.0.0.1:50000"] = Online
	// server.message = make(chan []byte, 1)
	// server.broadcastHandler = PlumtreeBroadcast
}

func (server *HeartbeatServer) start() {
	addr, err := net.ResolveUDPAddr("udp", server.host)
	if err != nil {
		log.Println("ResolveUDPAddr err")
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Println(server.host, "ListenUDP err", err)
		return
	}
	log.Println(server.host, "start")

	server.conn = conn

	message := make(chan []byte, 10)
	go receiveMessage(conn, message)
	go server.precessMessage(message)
	go server.beat()
}

func (server *HeartbeatServer) stop() {
	log.Infof(server.host, "Closing...")
	if err := server.conn.Close(); err != nil {
		log.Errorf("Error closing UDP connection: %v", err)
	}
}

func (server *HeartbeatServer) precessMessage(message chan []byte) {
	log.Println(server.host, "precessMessage")
	buf := <-message
	switch buf[0] {
	case Beat:
		server.replayBeat(buf)
	case RelayBeat:
		server.dealRelay(buf)
	case ReplyBeat:
		server.dealReply(buf)
	default:
		log.Println("unkonw udp type", buf[0])
	}
}

func (server *HeartbeatServer) beat() {
	time.Sleep(5 * time.Second)
	for {
		for _, target := range server.member {
			if server.host == target {
				continue
			}
			message := []byte{Beat}
			fmt.Println(server.host, "beat", target)
			sendUDPMessage(message, target)
			time.Sleep(1 * time.Second)
		}
	}
}

func (server *HeartbeatServer) replayBeat(message []byte) {

}

func (server *HeartbeatServer) dealRelay(message []byte) {

}

func (server *HeartbeatServer) dealReply(message []byte) {

}

func receiveMessage(conn *net.UDPConn, message chan []byte) {
	for {
		buf := make([]byte, 128)
		n, addr, err := conn.ReadFromUDP(buf) // 从UDP连接读取数据
		if err != nil {
			log.Println(conn.LocalAddr(), "ReadFromUDP err", addr, err)
			continue
		}

		res := string(buf[:n])
		message <- []byte(res)
	}
}

func sendUDPMessage(message []byte, target string) {
	conn, err := net.Dial("udp", target)
	if err != nil {
		log.Println("dial err", err)
		return
	}
	defer conn.Close()

	n, err := conn.Write(message)
	if err != nil {
		fmt.Println("Message sent to server err", n)
	}
}
