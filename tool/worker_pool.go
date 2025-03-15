package tool

import (
	"fmt"
	"sync"
)

// WorkerPool 结构
type WorkerPool struct {
	taskQueue   chan func()
	workerCount int
	wg          sync.WaitGroup
}

// 创建 WorkerPool
func NewWorkerPool(workerCount int) *WorkerPool {
	return &WorkerPool{
		taskQueue:   make(chan func()),
		workerCount: workerCount,
	}
}

// 启动 worker
func (wp *WorkerPool) Start() {
	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go func(id int) {
			defer wp.wg.Done()
			for task := range wp.taskQueue {
				fmt.Printf("Worker %d processing task\n", id)
				task()
			}
		}(i)
	}
}

// 提交任务
func (wp *WorkerPool) Submit(task func()) {
	wp.taskQueue <- task
}

// 关闭协程池
func (wp *WorkerPool) Shutdown() {
	close(wp.taskQueue)
	wp.wg.Wait() // 等待所有 worker 退出
}
