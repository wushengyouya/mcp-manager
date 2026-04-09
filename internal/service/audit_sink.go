package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
)

// AuditEntry 定义审计写入载荷
type AuditEntry struct {
	UserID       string
	Username     string
	Role         entity.Role
	Action       string
	ResourceType string
	ResourceID   string
	Detail       map[string]any
	IPAddress    string
	UserAgent    string
}

// AuditSink 定义最小审计接口
type AuditSink interface {
	Record(ctx context.Context, entry AuditEntry) error
}

// NoopAuditSink 定义空实现
type NoopAuditSink struct{}

// Record 执行空操作
func (NoopAuditSink) Record(ctx context.Context, entry AuditEntry) error {
	return nil
}

// DBAuditSink 定义数据库审计实现
type DBAuditSink struct {
	repo repository.AuditLogRepository
}

// NewDBAuditSink 创建数据库审计实现
func NewDBAuditSink(repo repository.AuditLogRepository) AuditSink {
	return &DBAuditSink{repo: repo}
}

// Record 写入审计日志
func (s *DBAuditSink) Record(ctx context.Context, entry AuditEntry) error {
	return s.repo.Create(ctx, &entity.AuditLog{
		ID:           uuid.NewString(),
		UserID:       entry.UserID,
		Username:     entry.Username,
		Action:       entry.Action,
		ResourceType: entry.ResourceType,
		ResourceID:   entry.ResourceID,
		Detail:       entry.Detail,
		IPAddress:    entry.IPAddress,
		UserAgent:    entry.UserAgent,
		CreatedAt:    time.Now(),
	})
}
