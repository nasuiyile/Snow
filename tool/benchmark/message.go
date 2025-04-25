package main

import (
	"math"
)

type Message struct {
	Id        string
	Cycle     int
	Size      int
	Target    string
	From      string
	Timestamp int
	Primary   bool
	MsgType   byte

	Num    int `json:"Num"`
	FanOut int `json:"FanOut"`
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
	Cycle          int
	BroadcastCount int
	FlowSum        int
	Reliability    float64
	LDT            int
	RMR            float64
	FlowInS        float64
	FlowOutS       float64
}

func staticticsCycle(message []Message) MessageCycle {
	cycle := MessageCycle{}
	// cycle.Id = k
	// cycle.BroadcastCount = len(v.getMessages())
	// 广播产生的总流量
	// cycle.FlowSum = v.totalSize

	// 过滤每个节点第一次收到消息的时间
	timeSet := make(map[string]int)
	for _, message := range message {
		if _, e := timeSet[message.Target]; !e {
			timeSet[message.Target] = message.Timestamp
		}
	}
	startTime := math.MaxInt
	endTime := math.MinInt
	for _, v := range timeSet {
		if startTime > v {
			startTime = v
		}
		if endTime < v {
			endTime = v
		}
	}
	cycle.LDT = endTime - startTime

	m := 0
	n := 0
	for _, message := range message {
		m += message.Size
		n = message.Num
	}
	cycle.RMR = (float64(m) / float64(n-1))

	// 统计有多少节点收到消息
	set := make(map[string]int)
	for _, m := range message {
		if _, e := set[m.Target]; !e {
			set[m.Target] = 0
		}
		set[m.Target]++
	}
	cycle.Reliability = float64(len(set)) / float64(n-1)

	// 统计节点的扇入扇出流量方差
	nodeSet := make(map[string]*MessageNode)
	for _, m := range message {
		if _, e := nodeSet[m.From]; !e {
			nodeSet[m.From] = &MessageNode{Node: m.From, FlowIn: 0, FlowOut: m.Size}
		} else {
			nodeSet[m.From].FlowOut += m.Size
		}
		if _, e := nodeSet[m.Target]; !e {
			nodeSet[m.Target] = &MessageNode{Node: m.Target, FlowIn: m.Size, FlowOut: 0}
		} else {
			nodeSet[m.Target].FlowIn += m.Size
		}
	}

	if len(nodeSet) > 0 {
		flowInSum := 0
		flowOutSum := 0
		flowInAvg := 0.0
		flowOutAvg := 0.0
		flowInS := 0.0
		flowOutS := 0.0
		for _, v := range nodeSet {
			flowInSum += v.FlowIn
			flowOutSum += v.FlowOut
		}
		flowInAvg = float64(flowInSum) / float64(len(nodeSet))
		flowOutAvg = float64(flowOutSum) / float64(len(nodeSet))
		for _, v := range nodeSet {
			flowInS += math.Pow(float64(v.FlowIn)-flowInAvg, 2)
			flowOutS += math.Pow(float64(v.FlowOut)-flowOutAvg, 2)
		}
		flowInS = float64(flowInS) / float64(len(nodeSet))
		flowOutS = float64(flowOutS) / float64(len(nodeSet))
		cycle.FlowInS = flowInS
		cycle.FlowOutS = flowOutS
	}

	return cycle
}
