package repository

import (
	"context"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"gorm.io/gorm"
)

// UserListFilter 定义用户列表过滤条件
type UserListFilter struct {
	Page     int
	PageSize int
	Role     string
	Active   *bool
}

// UserRepository 定义用户仓储接口
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

// userRepository 实现用户仓储。
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户仓储
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

// Create 创建用户记录
func (r *userRepository) Create(ctx context.Context, user *entity.User) error {
	if err := r.db.WithContext(ctx).Create(user).Error; err != nil {
		if isUniqueErr(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

// Update 更新用户记录
func (r *userRepository) Update(ctx context.Context, user *entity.User) error {
	if err := r.db.WithContext(ctx).Save(user).Error; err != nil {
		if isUniqueErr(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

// Delete 软删除指定用户
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

// GetByID 根据 ID 查询用户
func (r *userRepository) GetByID(ctx context.Context, id string) (*entity.User, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &user, nil
}

// GetByUsername 根据用户名查询用户
func (r *userRepository) GetByUsername(ctx context.Context, username string) (*entity.User, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).First(&user, "username = ?", username).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &user, nil
}

// GetByEmail 根据邮箱查询用户
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*entity.User, error) {
	var user entity.User
	if err := r.db.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &user, nil
}

// ExistsByUsername 判断用户名是否已存在
func (r *userRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	return exists(ctx, r.db, &entity.User{}, "username = ?", username)
}

// ExistsByEmail 判断邮箱是否已存在
func (r *userRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return exists(ctx, r.db, &entity.User{}, "email = ?", email)
}

// List 按过滤条件分页查询用户
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

// UpdateLastLogin 更新用户最后登录时间
func (r *userRepository) UpdateLastLogin(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", id).Update("last_login_at", at).Error
}

// UpdatePassword 更新用户密码并清理首次登录标记
func (r *userRepository) UpdatePassword(ctx context.Context, id, hashed string) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", id).Updates(map[string]any{
		"password":       hashed,
		"is_first_login": false,
	}).Error
}

// SetFirstLoginFalse 将首次登录标记设为 false
func (r *userRepository) SetFirstLoginFalse(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", id).Update("is_first_login", false).Error
}
