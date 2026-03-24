package repository

import (
	"context"

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
}

type mcpServiceRepository struct {
	db *gorm.DB
}

// NewMCPServiceRepository 创建服务仓储
func NewMCPServiceRepository(db *gorm.DB) MCPServiceRepository {
	return &mcpServiceRepository{db: db}
}

// Create 创建服务记录
func (r *mcpServiceRepository) Create(ctx context.Context, service *entity.MCPService) error {
	if err := r.db.WithContext(ctx).Create(service).Error; err != nil {
		if isUniqueErr(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

// Update 更新服务记录
func (r *mcpServiceRepository) Update(ctx context.Context, service *entity.MCPService) error {
	if err := r.db.WithContext(ctx).Save(service).Error; err != nil {
		if isUniqueErr(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
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
		query = query.Where("tags LIKE ?", "%"+filter.Tag+"%")
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

// UpdateStatus 更新服务运行状态字段
func (r *mcpServiceRepository) UpdateStatus(ctx context.Context, id string, status entity.ServiceStatus, failureCount int, lastError string) error {
	return r.db.WithContext(ctx).Model(&entity.MCPService{}).Where("id = ?", id).Updates(map[string]any{
		"status":        status,
		"failure_count": failureCount,
		"last_error":    lastError,
	}).Error
}
