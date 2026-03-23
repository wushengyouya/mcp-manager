package service

import (
	"context"
	"net/http"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// CreateUserInput 定义创建用户输入
type CreateUserInput struct {
	Username string
	Password string
	Email    string
	Role     entity.Role
}

// UpdateUserInput 定义更新用户输入
type UpdateUserInput struct {
	Email    string
	Role     entity.Role
	IsActive *bool
}

// UserService 定义用户业务接口
type UserService interface {
	Create(ctx context.Context, input CreateUserInput, actor AuditEntry) (*entity.User, error)
	Update(ctx context.Context, id string, input UpdateUserInput, actor AuditEntry) (*entity.User, error)
	Delete(ctx context.Context, id, currentUserID string, actor AuditEntry) error
	Get(ctx context.Context, id string) (*entity.User, error)
	List(ctx context.Context, filter repository.UserListFilter) ([]entity.User, int64, error)
}

type userService struct {
	users repository.UserRepository
	audit AuditSink
}

// NewUserService 创建用户服务
func NewUserService(users repository.UserRepository, audit AuditSink) UserService {
	if audit == nil {
		audit = NoopAuditSink{}
	}
	return &userService{users: users, audit: audit}
}

// Create 创建用户并写入审计日志
func (s *userService) Create(ctx context.Context, input CreateUserInput, actor AuditEntry) (*entity.User, error) {
	if exists, err := s.users.ExistsByUsername(ctx, input.Username); err != nil {
		return nil, err
	} else if exists {
		return nil, response.NewBizError(http.StatusConflict, response.CodeConflict, "用户名已存在", nil)
	}
	if exists, err := s.users.ExistsByEmail(ctx, input.Email); err != nil {
		return nil, err
	} else if exists {
		return nil, response.NewBizError(http.StatusConflict, response.CodeConflict, "邮箱已存在", nil)
	}
	hashed, err := appcrypto.HashPassword(input.Password)
	if err != nil {
		return nil, response.NewBizError(http.StatusBadRequest, response.CodeInvalidArgument, "密码不合法", err)
	}
	user := &entity.User{
		Username:     input.Username,
		Password:     hashed,
		Email:        input.Email,
		Role:         input.Role,
		IsActive:     true,
		IsFirstLogin: true,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}
	actor.ResourceID = user.ID
	actor.Action = "create_user"
	actor.ResourceType = "user"
	actor.Detail = map[string]any{"username": user.Username, "role": user.Role}
	_ = s.audit.Record(ctx, actor)
	return user, nil
}

// Update 更新用户基础信息
func (s *userService) Update(ctx context.Context, id string, input UpdateUserInput, actor AuditEntry) (*entity.User, error) {
	user, err := s.users.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if input.Email != "" && input.Email != user.Email {
		if exists, err := s.users.ExistsByEmail(ctx, input.Email); err != nil {
			return nil, err
		} else if exists {
			return nil, response.NewBizError(http.StatusConflict, response.CodeConflict, "邮箱已存在", nil)
		}
		user.Email = input.Email
	}
	if input.Role != "" {
		user.Role = input.Role
	}
	if input.IsActive != nil {
		user.IsActive = *input.IsActive
	}
	if err := s.users.Update(ctx, user); err != nil {
		return nil, err
	}
	actor.Action = "update_user"
	actor.ResourceType = "user"
	actor.ResourceID = user.ID
	actor.Detail = map[string]any{"email": user.Email, "role": user.Role, "is_active": user.IsActive}
	_ = s.audit.Record(ctx, actor)
	return user, nil
}

// Delete 删除指定用户，但禁止删除自己
func (s *userService) Delete(ctx context.Context, id, currentUserID string, actor AuditEntry) error {
	if id == currentUserID {
		return response.NewBizError(http.StatusBadRequest, response.CodeInvalidArgument, "不能删除自己", nil)
	}
	if err := s.users.Delete(ctx, id); err != nil {
		return err
	}
	actor.Action = "delete_user"
	actor.ResourceType = "user"
	actor.ResourceID = id
	_ = s.audit.Record(ctx, actor)
	return nil
}

// Get 查询单个用户
func (s *userService) Get(ctx context.Context, id string) (*entity.User, error) {
	return s.users.GetByID(ctx, id)
}

// List 分页查询用户列表
func (s *userService) List(ctx context.Context, filter repository.UserListFilter) ([]entity.User, int64, error) {
	return s.users.List(ctx, filter)
}
