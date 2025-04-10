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

type Command = byte

const (
	Stop Command = iota
)

var pServerMap map[string]PlumServer

type PlumServer struct {
	host     string
	listener net.Listener
	member   map[string]int
	message  map[string]int
}

func (server *PlumServer) init(port int) {
	server.host = fmt.Sprintf("127.0.0.1:%d", port)
	server.member = map[string]int{}
	server.member[server.host] = 1
}

func (server *PlumServer) start() {
	listener, err := net.Listen("tcp", server.host)
	if err != nil {
		log.Println("linten err", err)
		return
	}
	server.listener = listener
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("acccept err", err)
			break
		}

		go server.process(conn)
	}
}

func (server *PlumServer) stop() {
	server.listener.Close()
}

func (server *PlumServer) process(conn net.Conn) {
	defer conn.Close()
	for {
		reader := bufio.NewReader(conn)
		buf := make([]byte, 128)
		n, err := reader.Read(buf[:])
		if err != nil {
			log.Println("read err", err)
			break
		}
		res := string(buf[:n])
		log.Println(res)

		conn.Write([]byte("success"))

		if buf[0] == Stop {
			server.stop()
		}
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

func nextNodes(host string) []string {
	member := pServerMap[host].member

	memberList := make([]string, 0)
	for addr, _ := range member {
		if host != addr {
			memberList = append(memberList, addr)
		}
	}

	tAddr := make([]string, 0)
	for n := range randomNum(len(memberList), 3) {
		tAddr = append(tAddr, memberList[n])
	}
	return tAddr
}

func TestMessageClient(t *testing.T) {
	conn, err := net.Dial("tcp", "127.0.0.1:40000")
	if err != nil {
		log.Println("dial err", err)
		return
	}
	defer conn.Close()

	// _, err = conn.Write([]byte("hello world"))
	_, err = conn.Write([]byte{Stop})
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

func TestMessageServer(t *testing.T) {
	serverArr := make([]PlumServer, 0)
	for i := range 5 {
		server := new(PlumServer)
		server.init(40000 + i)
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
