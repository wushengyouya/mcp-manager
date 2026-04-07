package repository

import (
	"context"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ToolRepository 定义工具仓储接口
type ToolRepository interface {
	Create(ctx context.Context, tool *entity.Tool) error
	Update(ctx context.Context, tool *entity.Tool) error
	DeleteByService(ctx context.Context, serviceID string) (int64, error)
	GetByID(ctx context.Context, id string) (*entity.Tool, error)
	GetByServiceAndName(ctx context.Context, serviceID, name string) (*entity.Tool, error)
	ListByService(ctx context.Context, serviceID string) ([]entity.Tool, error)
	BatchUpsert(ctx context.Context, tools []entity.Tool) error
}

// toolRepository 实现工具仓储。
type toolRepository struct {
	db *gorm.DB
}

// NewToolRepository 创建工具仓储
func NewToolRepository(db *gorm.DB) ToolRepository {
	return &toolRepository{db: db}
}

// Create 创建工具记录
func (r *toolRepository) Create(ctx context.Context, tool *entity.Tool) error {
	return normalizeErr(r.db.WithContext(ctx).Create(tool).Error)
}

// Update 更新工具记录
func (r *toolRepository) Update(ctx context.Context, tool *entity.Tool) error {
	return normalizeErr(r.db.WithContext(ctx).Save(tool).Error)
}

// DeleteByService 按服务软删除其下全部工具
func (r *toolRepository) DeleteByService(ctx context.Context, serviceID string) (int64, error) {
	res := r.db.WithContext(ctx).Where("mcp_service_id = ?", serviceID).Delete(&entity.Tool{})
	return res.RowsAffected, res.Error
}

// GetByID 根据 ID 查询工具
func (r *toolRepository) GetByID(ctx context.Context, id string) (*entity.Tool, error) {
	var tool entity.Tool
	if err := r.db.WithContext(ctx).First(&tool, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &tool, nil
}

// GetByServiceAndName 按服务和名称查询工具
func (r *toolRepository) GetByServiceAndName(ctx context.Context, serviceID, name string) (*entity.Tool, error) {
	var tool entity.Tool
	if err := r.db.WithContext(ctx).Where("mcp_service_id = ? AND name = ?", serviceID, name).First(&tool).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &tool, nil
}

// ListByService 查询指定服务下的全部工具
func (r *toolRepository) ListByService(ctx context.Context, serviceID string) ([]entity.Tool, error) {
	var tools []entity.Tool
	if err := r.db.WithContext(ctx).Where("mcp_service_id = ?", serviceID).Order("name asc").Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}

// BatchUpsert 批量插入或更新工具元数据
func (r *toolRepository) BatchUpsert(ctx context.Context, tools []entity.Tool) error {
	if len(tools) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return normalizeErr(tx.Select("*").Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "mcp_service_id"},
				{Name: "name"},
			},
			TargetWhere: clause.Where{
				Exprs: []clause.Expression{
					clause.Expr{SQL: "deleted_at IS NULL"},
				},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"description",
				"input_schema",
				"is_enabled",
				"synced_at",
			}),
		}).Create(&tools).Error)
	})
}
