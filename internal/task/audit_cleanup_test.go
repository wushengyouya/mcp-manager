package task

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/stretchr/testify/require"
)

// mockAuditRepo 模拟审计日志仓储
type mockAuditRepo struct {
	mu          sync.Mutex
	deleteCount int64
	deletedAt   time.Time
}

// Create 实现审计仓储接口的空写入逻辑
func (m *mockAuditRepo) Create(_ context.Context, _ *entity.AuditLog) error {
	return nil
}

// List 实现审计仓储接口的空查询逻辑
func (m *mockAuditRepo) List(_ context.Context, _ repository.AuditListFilter) ([]entity.AuditLog, int64, error) {
	return nil, 0, nil
}

// DeleteOlderThan 记录清理任务的触发次数和时间
func (m *mockAuditRepo) DeleteOlderThan(_ context.Context, t time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCount++
	m.deletedAt = t
	return 5, nil
}

// getDeleteCount 返回清理动作被触发的次数
func (m *mockAuditRepo) getDeleteCount() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deleteCount
}

// TestAuditCleanupTask_StartStop 验证启动后 DeleteOlderThan 至少被调用一次
func TestAuditCleanupTask_StartStop(t *testing.T) {
	repo := &mockAuditRepo{}
	task := NewAuditCleanupTask(repo, 30, 10*time.Millisecond)
	task.Start()

	// 等待足够时间让 ticker 触发
	time.Sleep(80 * time.Millisecond)
	task.Stop()

	require.GreaterOrEqual(t, repo.getDeleteCount(), int64(1), "DeleteOlderThan 应至少被调用一次")
}

// TestAuditCleanupTask_DefaultInterval 验证 interval 为 0 时使用默认值
func TestAuditCleanupTask_DefaultInterval(t *testing.T) {
	repo := &mockAuditRepo{}
	task := NewAuditCleanupTask(repo, 30, 0)

	require.Equal(t, 24*time.Hour, task.interval, "默认 interval 应为 24 小时")
	task.Stop() // 未启动，但 Stop 不应 panic
}

// TestAuditCleanupTask_StopIdempotent 验证多次调用 Stop 不会 panic
func TestAuditCleanupTask_StopIdempotent(t *testing.T) {
	repo := &mockAuditRepo{}
	task := NewAuditCleanupTask(repo, 30, 10*time.Millisecond)
	task.Start()
	time.Sleep(30 * time.Millisecond)

	require.NotPanics(t, func() {
		task.Stop()
		task.Stop()
	}, "多次调用 Stop 不应 panic")
}
