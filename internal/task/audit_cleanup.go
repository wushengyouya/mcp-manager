package task

import (
	"context"
	"time"

	"github.com/mikasa/mcp-manager/internal/repository"
)

// AuditCleanupTask 定义审计清理任务。
type AuditCleanupTask struct {
	repo      repository.AuditLogRepository
	retention time.Duration
	interval  time.Duration
	stop      chan struct{}
}

// NewAuditCleanupTask 创建审计清理任务。
func NewAuditCleanupTask(repo repository.AuditLogRepository, retentionDays int, interval time.Duration) *AuditCleanupTask {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &AuditCleanupTask{
		repo:      repo,
		retention: time.Duration(retentionDays) * 24 * time.Hour,
		interval:  interval,
		stop:      make(chan struct{}),
	}
}

// Start 启动定时清理。
func (t *AuditCleanupTask) Start() {
	go func() {
		ticker := time.NewTicker(t.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_, _ = t.repo.DeleteOlderThan(context.Background(), time.Now().Add(-t.retention))
			case <-t.stop:
				return
			}
		}
	}()
}

// Stop 停止清理任务。
func (t *AuditCleanupTask) Stop() {
	select {
	case <-t.stop:
	default:
		close(t.stop)
	}
}
