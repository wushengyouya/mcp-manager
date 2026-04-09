package service

import (
	"context"
	"errors"
)

// AsyncAuditSink 使用后台队列异步落审计。
type AsyncAuditSink struct {
	queue *AsyncQueue
	next  AuditSink
}

// NewAsyncAuditSink 创建异步审计 sink。
func NewAsyncAuditSink(next AuditSink, workers, capacity int) *AsyncAuditSink {
	return &AsyncAuditSink{queue: NewAsyncQueue(workers, capacity), next: next}
}

// Record 将审计写入委派给后台 worker。
func (s *AsyncAuditSink) Record(ctx context.Context, entry AuditEntry) error {
	if s == nil || s.next == nil {
		return nil
	}
	cloned := entry
	err := s.queue.Submit(func(queueCtx context.Context) error {
		writeCtx := queueCtx
		if ctx != nil {
			writeCtx = context.Background()
		}
		return s.next.Record(writeCtx, cloned)
	})
	if errors.Is(err, ErrAsyncQueueFull) || errors.Is(err, ErrAsyncQueueStopped) {
		return s.next.Record(ctx, cloned)
	}
	return err
}

// Stop 停止后台 worker。
func (s *AsyncAuditSink) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.queue.Stop(ctx)
}

// Stats 返回异步队列统计。
func (s *AsyncAuditSink) Stats() AsyncQueueStats {
	if s == nil {
		return AsyncQueueStats{}
	}
	return s.queue.Stats()
}

// AsyncAlertService 使用后台队列异步发送告警。
type AsyncAlertService struct {
	queue *AsyncQueue
	next  AlertService
}

// NewAsyncAlertService 创建异步告警服务。
func NewAsyncAlertService(next AlertService, workers, capacity int) *AsyncAlertService {
	return &AsyncAlertService{queue: NewAsyncQueue(workers, capacity), next: next}
}

// NotifyServiceError 将告警请求放入后台队列。
func (s *AsyncAlertService) NotifyServiceError(ctx context.Context, serviceName, transportType, endpoint, reason string) error {
	if s == nil || s.next == nil {
		return nil
	}
	err := s.queue.Submit(func(queueCtx context.Context) error {
		notifyCtx := queueCtx
		if ctx != nil {
			notifyCtx = context.Background()
		}
		return s.next.NotifyServiceError(notifyCtx, serviceName, transportType, endpoint, reason)
	})
	if errors.Is(err, ErrAsyncQueueFull) || errors.Is(err, ErrAsyncQueueStopped) {
		return s.next.NotifyServiceError(ctx, serviceName, transportType, endpoint, reason)
	}
	return err
}

// Stop 停止后台 worker。
func (s *AsyncAlertService) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	return s.queue.Stop(ctx)
}

// Stats 返回异步队列统计。
func (s *AsyncAlertService) Stats() AsyncQueueStats {
	if s == nil {
		return AsyncQueueStats{}
	}
	return s.queue.Stats()
}
