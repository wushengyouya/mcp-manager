package repository

import (
	"context"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"gorm.io/gorm"
)

// AuditListFilter 定义审计查询条件
type AuditListFilter struct {
	Page         int
	PageSize     int
	UserID       string
	Action       string
	ResourceType string
	StartAt      *time.Time
	EndAt        *time.Time
}

// AuditLogRepository 定义审计仓储接口
type AuditLogRepository interface {
	Create(ctx context.Context, item *entity.AuditLog) error
	List(ctx context.Context, filter AuditListFilter) ([]entity.AuditLog, int64, error)
	DeleteOlderThan(ctx context.Context, t time.Time) (int64, error)
}

// auditLogRepository 实现审计日志仓储。
type auditLogRepository struct {
	db *gorm.DB
}

// NewAuditLogRepository 创建审计仓储
func NewAuditLogRepository(db *gorm.DB) AuditLogRepository {
	return &auditLogRepository{db: db}
}

// Create 创建审计日志
func (r *auditLogRepository) Create(ctx context.Context, item *entity.AuditLog) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// List 按过滤条件分页查询审计日志
func (r *auditLogRepository) List(ctx context.Context, filter AuditListFilter) ([]entity.AuditLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&entity.AuditLog{})
	if filter.UserID != "" {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.ResourceType != "" {
		query = query.Where("resource_type = ?", filter.ResourceType)
	}
	if filter.StartAt != nil {
		query = query.Where("created_at >= ?", *filter.StartAt)
	}
	if filter.EndAt != nil {
		query = query.Where("created_at <= ?", *filter.EndAt)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	var items []entity.AuditLog
	if err := query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// DeleteOlderThan 删除指定时间之前的审计日志
func (r *auditLogRepository) DeleteOlderThan(ctx context.Context, t time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("created_at < ?", t).Delete(&entity.AuditLog{})
	return res.RowsAffected, res.Error
}
