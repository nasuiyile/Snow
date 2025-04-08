package common

type MsgType = byte

// 定义枚举值
const (
	PingMsg MsgType = iota
	IndirectPingMsg
	AckRespMsg
	SuspectMsg
	AliveMsg
	DeadMsg
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

	UnicastMsg
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
	PingAction
)

type NodeState = byte

// TCP连接->广播自己存活->离开
const (
	NodePrepare   NodeState = iota //接收完TCP连接
	NodeSurvival                   //正常在Iptable中
	NodeSuspected                  //被怀疑离开
	NodeLeft                       //已经离开，稍后从列表中删除
)

const TimeLen = 8
const TagLen = 2
const HashLen = 32

var IpLen = 6
var Placeholder = TagLen + IpLen*2 + TimeLen
var Offset = 3000
