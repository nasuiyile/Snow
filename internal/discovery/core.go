package discovery

type ChangeType = byte

const (
	pingMsg ChangeType = iota
	indirectPingMsg
)

func initMsg(msg []byte) {
	//1 byte的类型，8byte的时间戳，然后是发消息节点的 ip加端口号

}

func NodeChange(msg []byte) {
	//
}
