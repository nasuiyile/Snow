package benchmark

type Message struct {
	Id        string
	Size      int
	Target    string
	From      string
	Timestamp int
	Primary   bool
}

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
