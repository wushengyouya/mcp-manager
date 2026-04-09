package service

import (
	"context"
	"errors"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
)

// HistorySink 定义调用历史写入接口。
type HistorySink interface {
	Record(ctx context.Context, item *entity.RequestHistory) error
}

type dbHistorySink struct {
	repo repository.RequestHistoryRepository
}

// NewDBHistorySink 创建数据库历史写入实现。
func NewDBHistorySink(repo repository.RequestHistoryRepository) HistorySink {
	return &dbHistorySink{repo: repo}
}

func (s *dbHistorySink) Record(ctx context.Context, item *entity.RequestHistory) error {
	return s.repo.Create(ctx, item)
}

// AsyncHistorySink 使用后台队列异步写入历史记录。
type AsyncHistorySink struct {
	queue *AsyncQueue
	next  HistorySink
}

// NewAsyncHistorySink 创建异步历史 sink。
func NewAsyncHistorySink(next HistorySink, workers, capacity int) *AsyncHistorySink {
	return &AsyncHistorySink{queue: NewAsyncQueue(workers, capacity), next: next}
}

// Record 将历史记录推入后台队列。
func (s *AsyncHistorySink) Record(ctx context.Context, item *entity.RequestHistory) error {
	if s == nil || s.next == nil {
		return nil
	}
	cloned := *item
	err := s.queue.Submit(func(queueCtx context.Context) error {
		writeCtx := queueCtx
		if ctx != nil {
			writeCtx = context.Background()
		}
		return s.next.Record(writeCtx, &cloned)
	})
	if errors.Is(err, ErrAsyncQueueFull) || errors.Is(err, ErrAsyncQueueStopped) {
		return s.next.Record(ctx, &cloned)
	}
	return err
}

// Stop 停止后台 worker 并 drain 队列。
func (s *AsyncHistorySink) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.queue.Stop(ctx)
}

// Stats 返回异步队列统计。
func (s *AsyncHistorySink) Stats() AsyncQueueStats {
	if s == nil {
		return AsyncQueueStats{}
	}
	return s.queue.Stats()
}
