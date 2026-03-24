package repository

import (
	"context"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"gorm.io/gorm"
)

// HistoryListFilter 定义历史查询条件
type HistoryListFilter struct {
	Page      int
	PageSize  int
	ServiceID string
	ToolName  string
	Status    string
	UserID    string
	IsAdmin   bool
	StartAt   *time.Time
	EndAt     *time.Time
}

// RequestHistoryRepository 定义历史仓储接口
type RequestHistoryRepository interface {
	Create(ctx context.Context, item *entity.RequestHistory) error
	GetByID(ctx context.Context, id string) (*entity.RequestHistory, error)
	List(ctx context.Context, filter HistoryListFilter) ([]entity.RequestHistory, int64, error)
}

type requestHistoryRepository struct {
	db *gorm.DB
}

// NewRequestHistoryRepository 创建历史仓储
func NewRequestHistoryRepository(db *gorm.DB) RequestHistoryRepository {
	return &requestHistoryRepository{db: db}
}

// Create 创建调用历史记录
func (r *requestHistoryRepository) Create(ctx context.Context, item *entity.RequestHistory) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// GetByID 根据 ID 查询调用历史
func (r *requestHistoryRepository) GetByID(ctx context.Context, id string) (*entity.RequestHistory, error) {
	var item entity.RequestHistory
	if err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &item, nil
}

// List 按过滤条件分页查询调用历史
func (r *requestHistoryRepository) List(ctx context.Context, filter HistoryListFilter) ([]entity.RequestHistory, int64, error) {
	query := r.db.WithContext(ctx).Model(&entity.RequestHistory{})
	if filter.ServiceID != "" {
		query = query.Where("mcp_service_id = ?", filter.ServiceID)
	}
	if filter.ToolName != "" {
		query = query.Where("tool_name = ?", filter.ToolName)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if !filter.IsAdmin {
		query = query.Where("user_id = ?", filter.UserID)
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
	var items []entity.RequestHistory
	if err := query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
