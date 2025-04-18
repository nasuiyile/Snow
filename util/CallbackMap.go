package util

import (
	"sync"
	"time"
)

type CallbackMap struct {
	m     map[int64]string // 存储实际数据
	mutex sync.RWMutex     // 读写锁，保证并发安全
	f     *func(s string)
}

func NewCallBackMap() *CallbackMap {
	tm := &CallbackMap{
		m:     make(map[int64]string),
		mutex: sync.RWMutex{},
	}
	return tm
}

func (cm *CallbackMap) Set(key int64, value string) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.m[key] = value
}

func (cm *CallbackMap) Delete(key int64) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	delete(cm.m, key)
}

// Get 获取键值对，并判断是否过期。如果过期则删除并返回空值。
func (cm *CallbackMap) Get(key int64) interface{} {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	if val, ok := cm.m[key]; ok {
		return val
	} else {

		return nil
	}
}

// Add 判断是否被写入过
func (cm *CallbackMap) Add(k int64, v string, f func(s string), timeout time.Duration) {
	cm.Set(k, v)
	time.AfterFunc(timeout, func() {
		cm.mutex.Lock()
		defer cm.mutex.Unlock()
		if _, ok := cm.m[k]; ok {
			go f(v)
			delete(cm.m, k)
		} else {
			return
		}
	})
}
