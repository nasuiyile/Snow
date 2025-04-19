package plumtree

import (
	"bufio"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"testing"
	"time"
)

type EventType = byte

const (
	Stop EventType = iota
	Start
	Broadcast
	Online
	Offline
)

type MessageType = byte

const (
	Normal MessageType = iota
	NeighborUp
	NeighborDown
)

type PlumServer struct {
	host             string
	listener         net.Listener
	member           map[string]byte
	message          chan []byte
	broadcastHandler func([]string, []byte)
}

func (server *PlumServer) init(port int) {
	server.host = fmt.Sprintf("127.0.0.1:%d", port)
	server.member = map[string]byte{}
	server.member[server.host] = Start
	server.member["127.0.0.1:20000"] = Online
	server.message = make(chan []byte, 1)
	server.broadcastHandler = PlumtreeBroadcast
}

func (server *PlumServer) start() {
	listener, err := net.Listen("tcp", server.host)
	if err != nil {
		log.Println(server.host, "linten err", err)
		return
	}
	server.listener = listener

	log.Println(server.host, "start")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(server.host, "acccept err", err)
			break
		}

		message := make(chan []byte, 1)
		go processMessage(conn, message)
		go server.messageHandle(message)
	}
}

func (server *PlumServer) stop() {
	server.listener.Close()
	log.Println(server.host, "stop")
}

func (server *PlumServer) Online() {
	message := []byte{Broadcast, NeighborUp}
	message = append(message, []byte(server.host)...)
	server.Broadcast(message)
}

func (server *PlumServer) Offline() {
	message := []byte{Broadcast, NeighborDown}
	message = append(message, []byte(server.host)...)
	server.Broadcast(message)
}

func (server *PlumServer) NeighborUp(message []byte) {
	host := string(message[2:])
	if state, e := server.member[host]; e && state == Online {
		return
	} else {
		server.member[host] = Online
		server.Broadcast(message)
	}
}

func (server *PlumServer) NeighborDown(message []byte) {
	host := string(message[2:])
	if _, e := server.member[host]; !e {
		server.member[host] = Offline
	}
	if server.member[host] == Offline {
		return
	} else {
		server.member[host] = Offline
		server.Broadcast(message)
	}
}

func (server *PlumServer) Broadcast(message []byte) {
	memberList := make([]string, 0)
	for addr, _ := range server.member {
		if server.host != addr {
			memberList = append(memberList, addr)
		}
	}
	server.broadcastHandler(memberList, message)
}

func processMessage(conn net.Conn, message chan []byte) {
	defer conn.Close()
	for {
		reader := bufio.NewReader(conn)
		buf := make([]byte, 512)
		n, err := reader.Read(buf[:])
		if err != nil {
			log.Println(conn.LocalAddr(), "read err", err)
			break
		}
		res := string(buf[:n])
		log.Println(conn.LocalAddr(), "process message", conn.RemoteAddr().String(), res)

		conn.Write([]byte("success"))

		message <- []byte(res)
		close(message)
	}
}

func (server *PlumServer) messageHandle(message chan []byte) {
	buf := <-message
	switch buf[0] {
	case Stop:
		server.stop()
	case Broadcast:
		switch buf[1] {
		case Normal:
		case NeighborUp:
			go server.NeighborUp(buf)
		case NeighborDown:
			go server.NeighborDown(buf)
		default:
			log.Println(string(buf))
		}
	case Online:
		go server.Online()
	case Offline:
		go server.Offline()
	default:
		log.Println(string(buf))
	}
	if buf[0] == Stop {
		server.stop()
	}
}

func PlumtreeBroadcast(memberList []string, message []byte) {
	nodes := nextNodes(memberList, 2)
	for _, node := range nodes {
		go sendMessage(message, node)
	}
}

func randomNum(count int, fanout int) []int {
	numMap := make(map[int]int)
	for i := range count {
		numMap[i] = 1
	}

	res := make([]int, 0)
	for range fanout {
		if len(numMap) == 0 {
			break
		}
		r := rand.IntN(len(numMap))
		j := 0
		for k, _ := range numMap {
			if j == r {
				res = append(res, k)
				delete(numMap, k)
				break
			}
			j++
		}
	}
	return res
}

func nextNodes(memberList []string, num int) []string {
	tAddr := make([]string, 0)
	for n := range randomNum(len(memberList), num) {
		tAddr = append(tAddr, memberList[n])
	}
	return tAddr
}

func sendMessage(message []byte, target string) {
	conn, err := net.Dial("tcp", target)
	if err != nil {
		log.Println("dial err", err)
		return
	}
	defer conn.Close()

	_, err = conn.Write(message)
	if err != nil {
		log.Println("write err", err)
		return
	}

	buf := [512]byte{}
	n, err := conn.Read(buf[:])
	if err != nil {
		log.Println("read err", err)
		return
	}
	fmt.Println(string(buf[:n]))
}

func TestMessageClient(t *testing.T) {
	message := []byte{Online}
	sendMessage(message, "127.0.0.1:40003")
}

func TestMessageServer(t *testing.T) {
	serverArr := make([]PlumServer, 0)
	for i := range 5 {
		server := new(PlumServer)
		server.init(20000 + i)
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
