package scripts

import (
	"context"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/logger"
)

// EnsureAdmin 确保默认管理员存在
func EnsureAdmin(ctx context.Context, repo repository.UserRepository, username, password, email string) error {
	exists, err := repo.ExistsByUsername(ctx, username)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	hashed, err := appcrypto.HashPassword(password)
	if err != nil {
		return err
	}
	if err := repo.Create(ctx, &entity.User{
		Username:     username,
		Password:     hashed,
		Email:        email,
		Role:         entity.RoleAdmin,
		IsActive:     true,
		IsFirstLogin: true,
	}); err != nil {
		return err
	}
	logger.S().Warnf("已初始化默认管理员账号 %s，请首次登录后立即修改密码", username)
	return nil
}
