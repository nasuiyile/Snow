package plumtree

import (
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type PlumServer struct {
	member  map[string]int
	message map[string]int
}

var pServerMap map[string]PlumServer

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

func plumBroadcast(host string, route *url.URL) {
	nodes := nextNodes(host)
	for _, node := range nodes {
		http.Get("http://" + node + route.RequestURI())
	}
}

func sendMessage(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	log.Println(host, "sendMessage")
	msg := r.URL.Query().Get("msg")
	r.URL.RequestURI()
	if _, exisit := pServerMap[host].message[msg]; !exisit {
		pServerMap[host].message[msg] = 1
		plumBroadcast(host, r.URL)
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

func TestServer(t *testing.T) {
	pServerMap = make(map[string]PlumServer)
	pMember := make(map[string]int)
	for i := range 5 {
		lAttr := fmt.Sprintf("localhost:%d", 40000+i)
		pMember[lAttr] = 1
	}

	for k, _ := range pMember {
		pServerMap[k] = PlumServer{member: pMember, message: make(map[string]int)}
	}

	http.HandleFunc("/applyLeave", applyLeave)
	http.HandleFunc("/sendMessage", sendMessage)
	for k, _ := range pServerMap {
		go startServer(k)
	}

	for {
		time.Sleep(1 * time.Second)
		time.Sleep(1 * time.Second)
	}
}

func startServer(lAttr string) {
	log.Println("http start ", lAttr)
	if err := http.ListenAndServe(lAttr, nil); err != nil {
		log.Printf("Error starting server: %s\n", err)
	}
}

func applyLeave(w http.ResponseWriter, r *http.Request) {

}
