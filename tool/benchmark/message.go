package benchmark

type Message struct {
	Id        string
	Size      int
	Target    string
	From      string
	Timestamp int
	Primary   bool
}

// 每个节点统计信息
type MessageNode struct {
	Node       string
	FanIn      int
	FanOut     int
	FlowIn     int
	FlowOut    int
	FlowInS    float64
	FlowOutS   float64
	FlowInAvg  float64
	FlowOutAvg float64
}

// 每个轮次统计统计信息
type MessageCycle struct {
	Id           string
	MessageCount int
	FlowSum      int
	LDT          int
	RMR          float64
}
