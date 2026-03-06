package repository

import (
	"context"
	"errors"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"gorm.io/gorm"
)

var (
	// ErrNotFound 表示资源不存在。
	ErrNotFound = errors.New("resource not found")
	// ErrAlreadyExists 表示资源已存在。
	ErrAlreadyExists = errors.New("resource already exists")
)

// UserListFilter 定义用户列表过滤条件。
type UserListFilter struct {
	Page     int
	PageSize int
	Role     string
	Active   *bool
}

// UserRepository 定义用户仓储接口。
type UserRepository interface {
	Create(ctx context.Context, user *entity.User) error
	Update(ctx context.Context, user *entity.User) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*entity.User, error)
	GetByUsername(ctx context.Context, username string) (*entity.User, error)
	GetByEmail(ctx context.Context, email string) (*entity.User, error)
	ExistsByUsername(ctx context.Context, username string) (bool, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	List(ctx context.Context, filter UserListFilter) ([]entity.User, int64, error)
	UpdateLastLogin(ctx context.Context, id string, at time.Time) error
	UpdatePassword(ctx context.Context, id, hashed string) error
	SetFirstLoginFalse(ctx context.Context, id string) error
}

type userRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户仓储。
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(ctx context.Context, user *entity.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		if isUniqueErr(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (r *userRepository) Update(ctx context.Context, user *entity.User) error {
	if err := r.db.WithContext(ctx).Save(user).Error; err != nil {
		if isUniqueErr(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (r *userRepository) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Delete(&entity.User{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*entity.User, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &user, nil
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*entity.User, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).First(&user, "username = ?", username).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &user, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*entity.User, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &user, nil
}

func (r *userRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	return exists(ctx, r.db, &entity.User{}, "username = ?", username)
}

func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return exists(ctx, r.db, &entity.User{}, "email = ?", email)
}

func (r *userRepository) List(ctx context.Context, filter UserListFilter) ([]entity.User, int64, error) {
	query := r.db.WithContext(ctx).Model(&entity.User{})
	if filter.Role != "" {
		query = query.Where("role = ?", filter.Role)
	}
	if filter.Active != nil {
		query = query.Where("is_active = ?", *filter.Active)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	var users []entity.User
	if err := query.Order("created_at desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (r *userRepository) UpdateLastLogin(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", id).Update("last_login_at", at).Error
}

func (r *userRepository) UpdatePassword(ctx context.Context, id, hashed string) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", id).Updates(map[string]any{
		"password":       hashed,
		"is_first_login": false,
	}).Error
}

func (r *userRepository) SetFirstLoginFalse(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", id).Update("is_first_login", false).Error
}

// MCPServiceListFilter 定义服务过滤条件。
type MCPServiceListFilter struct {
	Page          int
	PageSize      int
	TransportType string
	Tag           string
}

// MCPServiceRepository 定义服务仓储接口。
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

// NewMCPServiceRepository 创建服务仓储。
func NewMCPServiceRepository(db *gorm.DB) MCPServiceRepository {
	return &mcpServiceRepository{db: db}
}

func (r *mcpServiceRepository) Create(ctx context.Context, service *entity.MCPService) error {
	if err := r.db.WithContext(ctx).Create(service).Error; err != nil {
		if isUniqueErr(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (r *mcpServiceRepository) Update(ctx context.Context, service *entity.MCPService) error {
	if err := r.db.WithContext(ctx).Save(service).Error; err != nil {
		if isUniqueErr(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

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

func (r *mcpServiceRepository) GetByID(ctx context.Context, id string) (*entity.MCPService, error) {
	var item entity.MCPService
	if err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &item, nil
}

func (r *mcpServiceRepository) GetByName(ctx context.Context, name string) (*entity.MCPService, error) {
	var item entity.MCPService
	if err := r.db.WithContext(ctx).First(&item, "name = ?", name).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &item, nil
}

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

func (r *mcpServiceRepository) UpdateStatus(ctx context.Context, id string, status entity.ServiceStatus, failureCount int, lastError string) error {
	return r.db.WithContext(ctx).Model(&entity.MCPService{}).Where("id = ?", id).Updates(map[string]any{
		"status":        status,
		"failure_count": failureCount,
		"last_error":    lastError,
	}).Error
}

// ToolRepository 定义工具仓储接口。
type ToolRepository interface {
	Create(ctx context.Context, tool *entity.Tool) error
	Update(ctx context.Context, tool *entity.Tool) error
	DeleteByService(ctx context.Context, serviceID string) error
	GetByID(ctx context.Context, id string) (*entity.Tool, error)
	GetByServiceAndName(ctx context.Context, serviceID, name string) (*entity.Tool, error)
	ListByService(ctx context.Context, serviceID string) ([]entity.Tool, error)
	BatchUpsert(ctx context.Context, tools []entity.Tool) error
}

type toolRepository struct {
	db *gorm.DB
}

// NewToolRepository 创建工具仓储。
func NewToolRepository(db *gorm.DB) ToolRepository {
	return &toolRepository{db: db}
}

func (r *toolRepository) Create(ctx context.Context, tool *entity.Tool) error {
	return r.db.WithContext(ctx).Create(tool).Error
}

func (r *toolRepository) Update(ctx context.Context, tool *entity.Tool) error {
	return r.db.WithContext(ctx).Save(tool).Error
}

func (r *toolRepository) DeleteByService(ctx context.Context, serviceID string) error {
	return r.db.WithContext(ctx).Where("mcp_service_id = ?", serviceID).Delete(&entity.Tool{}).Error
}

func (r *toolRepository) GetByID(ctx context.Context, id string) (*entity.Tool, error) {
	var tool entity.Tool
	if err := r.db.WithContext(ctx).First(&tool, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &tool, nil
}

func (r *toolRepository) GetByServiceAndName(ctx context.Context, serviceID, name string) (*entity.Tool, error) {
	var tool entity.Tool
	if err := r.db.WithContext(ctx).Where("mcp_service_id = ? AND name = ?", serviceID, name).First(&tool).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &tool, nil
}

func (r *toolRepository) ListByService(ctx context.Context, serviceID string) ([]entity.Tool, error) {
	var tools []entity.Tool
	if err := r.db.WithContext(ctx).Where("mcp_service_id = ?", serviceID).Order("name asc").Find(&tools).Error; err != nil {
		return nil, err
	}
	return tools, nil
}

func (r *toolRepository) BatchUpsert(ctx context.Context, tools []entity.Tool) error {
	for _, tool := range tools {
		var existing entity.Tool
		err := r.db.WithContext(ctx).Where("mcp_service_id = ? AND name = ?", tool.MCPServiceID, tool.Name).First(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := r.db.WithContext(ctx).Create(&tool).Error; err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			existing.Description = tool.Description
			existing.InputSchema = tool.InputSchema
			existing.IsEnabled = tool.IsEnabled
			existing.SyncedAt = tool.SyncedAt
			if err := r.db.WithContext(ctx).Save(&existing).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// HistoryListFilter 定义历史查询条件。
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

// RequestHistoryRepository 定义历史仓储接口。
type RequestHistoryRepository interface {
	Create(ctx context.Context, item *entity.RequestHistory) error
	GetByID(ctx context.Context, id string) (*entity.RequestHistory, error)
	List(ctx context.Context, filter HistoryListFilter) ([]entity.RequestHistory, int64, error)
}

type requestHistoryRepository struct {
	db *gorm.DB
}

// NewRequestHistoryRepository 创建历史仓储。
func NewRequestHistoryRepository(db *gorm.DB) RequestHistoryRepository {
	return &requestHistoryRepository{db: db}
}

func (r *requestHistoryRepository) Create(ctx context.Context, item *entity.RequestHistory) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *requestHistoryRepository) GetByID(ctx context.Context, id string) (*entity.RequestHistory, error) {
	var item entity.RequestHistory
	if err := r.db.WithContext(ctx).First(&item, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &item, nil
}

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

// AuditListFilter 定义审计查询条件。
type AuditListFilter struct {
	Page         int
	PageSize     int
	UserID       string
	Action       string
	ResourceType string
	StartAt      *time.Time
	EndAt        *time.Time
}

// AuditLogRepository 定义审计仓储接口。
type AuditLogRepository interface {
	Create(ctx context.Context, item *entity.AuditLog) error
	List(ctx context.Context, filter AuditListFilter) ([]entity.AuditLog, int64, error)
	DeleteOlderThan(ctx context.Context, t time.Time) (int64, error)
}

type auditLogRepository struct {
	db *gorm.DB
}

// NewAuditLogRepository 创建审计仓储。
func NewAuditLogRepository(db *gorm.DB) AuditLogRepository {
	return &auditLogRepository{db: db}
}

func (r *auditLogRepository) Create(ctx context.Context, item *entity.AuditLog) error {
	return r.db.WithContext(ctx).Create(item).Error
}

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

func (r *auditLogRepository) DeleteOlderThan(ctx context.Context, t time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("created_at < ?", t).Delete(&entity.AuditLog{})
	return res.RowsAffected, res.Error
}

func normalizeErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "UNIQUE constraint failed")
}

func exists(ctx context.Context, db *gorm.DB, model any, query string, args ...any) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(model).Where(query, args...).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	return page, pageSize
}

func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && stringIndex(s, substr) >= 0)
}

func stringIndex(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}
