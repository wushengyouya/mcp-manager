package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	// ErrAsyncQueueFull 表示异步队列已满。
	ErrAsyncQueueFull = errors.New("async queue full")
	// ErrAsyncQueueStopped 表示异步队列已停止。
	ErrAsyncQueueStopped = errors.New("async queue stopped")
)

// AsyncQueueStats 描述异步队列当前状态。
type AsyncQueueStats struct {
	Pending  int   `json:"pending"`
	Capacity int   `json:"capacity"`
	Errors   int64 `json:"errors"`
}

// AsyncQueue 提供带 drain 的后台任务队列。
type AsyncQueue struct {
	jobs    chan func(context.Context) error
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	pending atomic.Int64
	errors  atomic.Int64
	closed  atomic.Bool
}

// NewAsyncQueue 创建后台任务队列。
func NewAsyncQueue(workers, capacity int) *AsyncQueue {
	if workers <= 0 {
		workers = 1
	}
	if capacity <= 0 {
		capacity = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	q := &AsyncQueue{
		jobs:   make(chan func(context.Context) error, capacity),
		ctx:    ctx,
		cancel: cancel,
	}
	q.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go q.run()
	}
	return q
}

func (q *AsyncQueue) run() {
	defer q.wg.Done()
	for job := range q.jobs {
		if job == nil {
			q.pending.Add(-1)
			continue
		}
		if err := job(q.ctx); err != nil {
			q.errors.Add(1)
		}
		q.pending.Add(-1)
	}
}

// Submit 尝试将任务放入后台队列。
func (q *AsyncQueue) Submit(job func(context.Context) error) error {
	if q == nil || job == nil {
		return nil
	}
	if q.closed.Load() {
		return ErrAsyncQueueStopped
	}
	select {
	case q.jobs <- job:
		q.pending.Add(1)
		return nil
	default:
		return ErrAsyncQueueFull
	}
}

// Stop 停止队列并等待已入队任务完成。
func (q *AsyncQueue) Stop(_ context.Context) error {
	if q == nil {
		return nil
	}
	if q.closed.CompareAndSwap(false, true) {
		close(q.jobs)
		q.wg.Wait()
		q.cancel()
	}
	return nil
}

// Stats 返回当前队列状态。
func (q *AsyncQueue) Stats() AsyncQueueStats {
	if q == nil {
		return AsyncQueueStats{}
	}
	return AsyncQueueStats{
		Pending:  int(q.pending.Load()),
		Capacity: cap(q.jobs),
		Errors:   q.errors.Load(),
	}
}
