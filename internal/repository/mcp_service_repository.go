package repository

import (
	"context"
	"encoding/json"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"gorm.io/gorm"
)

// MCPServiceListFilter 定义服务过滤条件
type MCPServiceListFilter struct {
	Page          int
	PageSize      int
	TransportType string
	Tag           string
}

// MCPServiceRepository 定义服务仓储接口
type MCPServiceRepository interface {
	Create(ctx context.Context, service *entity.MCPService) error
	Update(ctx context.Context, service *entity.MCPService) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*entity.MCPService, error)
	GetByName(ctx context.Context, name string) (*entity.MCPService, error)
	List(ctx context.Context, filter MCPServiceListFilter) ([]entity.MCPService, int64, error)
	UpdateStatus(ctx context.Context, id string, status entity.ServiceStatus, failureCount int, lastError string) error
	ResetConnectionStatuses(ctx context.Context) (int64, error)
}

// mcpServiceRepository 实现 MCP 服务仓储。
type mcpServiceRepository struct {
	db *gorm.DB
}

// NewMCPServiceRepository 创建服务仓储
func NewMCPServiceRepository(db *gorm.DB) MCPServiceRepository {
	return &mcpServiceRepository{db: db}
}

// Create 创建服务记录
func (r *mcpServiceRepository) Create(ctx context.Context, service *entity.MCPService) error {
	return normalizeErr(r.db.WithContext(ctx).Create(service).Error)
}

// Update 更新服务记录
func (r *mcpServiceRepository) Update(ctx context.Context, service *entity.MCPService) error {
	return normalizeErr(r.db.WithContext(ctx).Save(service).Error)
}

// Delete 软删除指定服务
func (r *mcpServiceRepository) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&entity.MCPService{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetByID 根据 ID 查询服务
func (r *mcpServiceRepository) GetByID(ctx context.Context, id string) (*entity.MCPService, error) {
	var item entity.MCPService
	if err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &item, nil
}

// GetByName 根据名称查询服务
func (r *mcpServiceRepository) GetByName(ctx context.Context, name string) (*entity.MCPService, error) {
	var item entity.MCPService
	if err := r.db.WithContext(ctx).First(&item, "name = ?", name).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &item, nil
}

// List 按过滤条件分页查询服务
func (r *mcpServiceRepository) List(ctx context.Context, filter MCPServiceListFilter) ([]entity.MCPService, int64, error) {
	query := r.db.WithContext(ctx).Model(&entity.MCPService{})
	if filter.TransportType != "" {
		query = query.Where("transport_type = ?", filter.TransportType)
	}
	if filter.Tag != "" {
		query = applyTagExactMatchFilter(query, filter.Tag)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	var items []entity.MCPService
	if err := query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func applyTagExactMatchFilter(query *gorm.DB, tag string) *gorm.DB {
	tagJSON, _ := json.Marshal([]string{tag})
	switch query.Dialector.Name() {
	case "postgres":
		return query.Where("COALESCE(tags, '[]'::jsonb) @> ?::jsonb", string(tagJSON))
	default:
		return query.Where("EXISTS (SELECT 1 FROM json_each(COALESCE(tags, '[]')) WHERE json_each.value = ?)", tag)
	}
}

// UpdateStatus 更新服务运行状态字段
func (r *mcpServiceRepository) UpdateStatus(ctx context.Context, id string, status entity.ServiceStatus, failureCount int, lastError string) error {
	return r.db.WithContext(ctx).Model(&entity.MCPService{}).Where("id = ?", id).Updates(map[string]any{
		"status":        status,
		"failure_count": failureCount,
		"last_error":    lastError,
	}).Error
}

// ResetConnectionStatuses 将依赖进程内运行态的连接状态重置为安全持久化状态。
func (r *mcpServiceRepository) ResetConnectionStatuses(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&entity.MCPService{}).
		Where("status IN ?", []entity.ServiceStatus{entity.ServiceStatusConnected, entity.ServiceStatusConnecting}).
		Updates(map[string]any{
			"status":        entity.ServiceStatusDisconnected,
			"failure_count": 0,
			"last_error":    "",
		})
	return result.RowsAffected, result.Error
}
