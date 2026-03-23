package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"time"

	"github.com/mikasa/mcp-manager/internal/repository"
)

// AuditService 定义增强审计服务
type AuditService interface {
	Record(ctx context.Context, entry AuditEntry) error
	List(ctx context.Context, filter repository.AuditListFilter) ([]map[string]any, int64, error)
	ExportCSV(ctx context.Context, filter repository.AuditListFilter) ([]byte, error)
}

type auditService struct {
	sink AuditSink
	repo repository.AuditLogRepository
}

// NewAuditService 创建增强审计服务
func NewAuditService(sink AuditSink, repo repository.AuditLogRepository) AuditService {
	return &auditService{sink: sink, repo: repo}
}

// Record 透传审计记录写入
func (s *auditService) Record(ctx context.Context, entry AuditEntry) error {
	return s.sink.Record(ctx, entry)
}

// List 查询审计日志并转换为统一输出结构
func (s *auditService) List(ctx context.Context, filter repository.AuditListFilter) ([]map[string]any, int64, error) {
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"id":            item.ID,
			"user_id":       item.UserID,
			"username":      item.Username,
			"action":        item.Action,
			"resource_type": item.ResourceType,
			"resource_id":   item.ResourceID,
			"detail":        item.Detail,
			"ip_address":    item.IPAddress,
			"user_agent":    item.UserAgent,
			"created_at":    item.CreatedAt,
		})
	}
	return out, total, nil
}

// ExportCSV 导出符合过滤条件的审计日志 CSV
func (s *auditService) ExportCSV(ctx context.Context, filter repository.AuditListFilter) ([]byte, error) {
	items, _, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(nil)
	w := csv.NewWriter(buf)
	_ = w.Write([]string{"id", "username", "action", "resource_type", "resource_id", "detail", "created_at"})
	for _, item := range items {
		raw, _ := json.Marshal(item.Detail)
		_ = w.Write([]string{item.ID, item.Username, item.Action, item.ResourceType, item.ResourceID, string(raw), item.CreatedAt.Format(time.RFC3339)})
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
