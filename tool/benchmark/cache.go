package benchmark

import (
	"math"
	"sync"
)

type MessageCache struct {
	mu        sync.RWMutex
	totalSize int
	startTime int
	endTime   int
	messages  []Message
}

func (cache *MessageCache) put(message Message) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	time := message.Timestamp
	if cache.startTime > time {
		cache.startTime = time
	}
	if cache.endTime < time {
		cache.endTime = time
	}

	cache.messages = append(cache.messages, message)
	cache.totalSize += message.Size
}

func (cache *MessageCache) getNodes() []*MessageNode {
	nodeMap := make(map[string]*MessageNode, 0)
	nodeArr := make([]*MessageNode, 0)
	// 统计每个节点的消息量
	for _, message := range cache.getMessages() {
		if v, e := nodeMap[message.From]; e {
			v.FanOut++
			v.FlowOut += message.Size
		} else {
			nodeMap[message.From] = &MessageNode{Node: message.From, FanIn: 0, FanOut: 1, FlowIn: 0, FlowOut: message.Size}
			nodeArr = append(nodeArr, nodeMap[message.From])
		}
		if v, e := nodeMap[message.Target]; e {
			v.FanIn++
			v.FlowIn += message.Size
		} else {
			nodeMap[message.Target] = &MessageNode{Node: message.Target, FanIn: 1, FanOut: 0, FlowIn: message.Size, FlowOut: 0}
			nodeArr = append(nodeArr, nodeMap[message.Target])
		}
	}
	return nodeArr
}

func (cache *MessageCache) getMessages() []Message {
	values := make([]Message, 0)
	values = append(values, cache.messages...)
	return values
}

func (cache *MessageCache) clearAll() {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	cache.messages = make([]Message, 0)
}

func CreateMessageCache() *MessageCache {
	return &MessageCache{
		startTime: math.MaxInt,
		endTime:   math.MinInt,
		messages:  make([]Message, 0),
	}
}
