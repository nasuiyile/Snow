package common

type MsgType = byte

// 定义枚举值
const (
	pingMsg MsgType = iota
	indirectPingMsg
	ackRespMsg
	suspectMsg
	aliveMsg
	deadMsg
	//节点状态改变
	NodeChange
	//这是消息的发送方式
	ColoringMsg
	RegularMsg     //普通的消息
	ReliableMsg    //可靠消息
	ReliableMsgAck //可靠消息回执
	GossipMsg      //gossip消息
	//plumtree
	EagerPush //急迫推送
	LazyPush  //发送IHAVE
	Graft     //收到IHAVE之后，发现自己没有，所以进行主动的拉取
	Prune     //用于修剪，自己已经收到了EagerPush

)

type MsgAction = byte

const (
	UserMsg MsgAction = iota
	ApplyJoin
	JoinStateSync
	NodeJoin
	NodeLeave   //这是自己请求离开的方法
	ReportLeave //这是被别人上报的节点离开方法
	RegularStateSync
	IHAVE //lazypush的发送内容
)
const TimeLen = 8
const TagLen = 2
const HashLen = 32

var IpLen = 6

const Placeholder = 1 + 1 + 6 + 6 + 8
