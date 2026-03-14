package service

import (
	"context"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/response"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAuthTest 初始化内存数据库、仓储、JWT 服务和 AuthService，并预置一个测试用户。
func setupAuthTest(t *testing.T) (AuthService, *appcrypto.JWTService, repository.UserRepository, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&entity.User{}, &entity.AuditLog{}))

	userRepo := repository.NewUserRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	auditSink := NewDBAuditSink(auditRepo)

	jwtSvc := appcrypto.NewJWTService("test-secret-key-for-unit-tests", "test-issuer",
		15*time.Minute, 24*time.Hour, nil)

	// 预置测试用户
	hashed, err := appcrypto.HashPassword("correct-password")
	require.NoError(t, err)
	testUser := &entity.User{
		Username: "testuser",
		Password: hashed,
		Email:    "test@example.com",
		Role:     entity.RoleAdmin,
		IsActive: true,
	}
	require.NoError(t, userRepo.Create(context.Background(), testUser))

	svc := NewAuthService(userRepo, jwtSvc, auditSink)
	return svc, jwtSvc, userRepo, db
}

// TestAuthService_Login 使用表驱动测试验证登录的各种场景。
func TestAuthService_Login(t *testing.T) {
	svc, _, userRepo, _ := setupAuthTest(t)
	ctx := context.Background()

	// 创建一个被禁用的用户：先创建再禁用，避免 GORM default:true 覆盖 false 零值
	hashed, err := appcrypto.HashPassword("disabled-pass")
	require.NoError(t, err)
	disabledUser := &entity.User{
		Username: "disabled",
		Password: hashed,
		Email:    "disabled@example.com",
		Role:     entity.RoleReadonly,
		IsActive: true,
	}
	require.NoError(t, userRepo.Create(ctx, disabledUser))
	isActive := false
	_, err = NewUserService(userRepo, NoopAuditSink{}).Update(ctx, disabledUser.ID, UpdateUserInput{IsActive: &isActive}, AuditEntry{})
	require.NoError(t, err)

	tests := []struct {
		name     string
		username string
		password string
		wantCode int  // 期望的 BizError Code，0 表示期望成功
		wantErr  bool // 是否期望返回错误
	}{
		{
			name:     "成功登录",
			username: "testuser",
			password: "correct-password",
			wantCode: 0,
			wantErr:  false,
		},
		{
			name:     "密码错误",
			username: "testuser",
			password: "wrong-password",
			wantCode: response.CodeUnauthorized,
			wantErr:  true,
		},
		{
			name:     "用户不存在",
			username: "nonexistent",
			password: "any-password",
			wantCode: response.CodeUnauthorized,
			wantErr:  true,
		},
		{
			name:     "用户已禁用",
			username: "disabled",
			password: "disabled-pass",
			wantCode: response.CodeForbidden,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pair, user, err := svc.Login(ctx, tt.username, tt.password, "127.0.0.1", "test-agent")
			if tt.wantErr {
				require.Error(t, err)
				var bizErr *response.BizError
				require.ErrorAs(t, err, &bizErr)
				require.Equal(t, tt.wantCode, bizErr.Code)
				require.Nil(t, pair)
				require.Nil(t, user)
			} else {
				require.NoError(t, err)
				require.NotNil(t, pair)
				require.NotEmpty(t, pair.AccessToken)
				require.NotEmpty(t, pair.RefreshToken)
				require.NotNil(t, user)
				require.Equal(t, tt.username, user.Username)
				require.NotNil(t, user.LastLoginAt)
			}
		})
	}
}

// TestAuthService_Logout 验证登出后令牌被加入黑名单。
func TestAuthService_Logout(t *testing.T) {
	svc, jwtSvc, _, _ := setupAuthTest(t)
	ctx := context.Background()

	// 先登录获取令牌
	pair, _, err := svc.Login(ctx, "testuser", "correct-password", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	// 登出
	err = svc.Logout(ctx, pair.AccessToken, pair.RefreshToken, "user-id", "testuser", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	// 验证令牌已在黑名单中
	_, err = jwtSvc.ParseToken(pair.AccessToken, appcrypto.TokenTypeAccess)
	require.Error(t, err)

	_, err = jwtSvc.ParseToken(pair.RefreshToken, appcrypto.TokenTypeRefresh)
	require.Error(t, err)
}

// TestAuthService_Refresh 验证刷新令牌返回新的令牌对，旧 refresh token 被黑名单。
func TestAuthService_Refresh(t *testing.T) {
	svc, jwtSvc, _, _ := setupAuthTest(t)
	ctx := context.Background()

	// 先登录获取令牌
	pair, _, err := svc.Login(ctx, "testuser", "correct-password", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	// 刷新
	newPair, err := svc.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)
	require.NotNil(t, newPair)
	require.NotEmpty(t, newPair.AccessToken)
	require.NotEmpty(t, newPair.RefreshToken)
	// 旧 refresh token 已不可用
	_, err = jwtSvc.ParseToken(pair.RefreshToken, appcrypto.TokenTypeRefresh)
	require.Error(t, err)

	// 新令牌可用
	_, err = jwtSvc.ParseToken(newPair.AccessToken, appcrypto.TokenTypeAccess)
	require.NoError(t, err)
}

// TestAuthService_Refresh_InvalidToken 验证无效的 refresh token 返回错误。
func TestAuthService_Refresh_InvalidToken(t *testing.T) {
	svc, _, _, _ := setupAuthTest(t)
	ctx := context.Background()

	_, err := svc.Refresh(ctx, "invalid-token-string")
	require.Error(t, err)
	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeUnauthorized, bizErr.Code)
}

// TestAuthService_ChangePassword_Success 验证成功修改密码。
func TestAuthService_ChangePassword_Success(t *testing.T) {
	svc, _, userRepo, _ := setupAuthTest(t)
	ctx := context.Background()

	// 查找预置用户
	user, err := userRepo.GetByUsername(ctx, "testuser")
	require.NoError(t, err)

	// 修改密码
	err = svc.ChangePassword(ctx, user.ID, "correct-password", "new-password-123", "testuser", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	// 用旧密码登录失败
	_, _, err = svc.Login(ctx, "testuser", "correct-password", "127.0.0.1", "test-agent")
	require.Error(t, err)

	// 用新密码登录成功
	pair, _, err := svc.Login(ctx, "testuser", "new-password-123", "127.0.0.1", "test-agent")
	require.NoError(t, err)
	require.NotNil(t, pair)
}

// TestAuthService_ChangePassword_WrongOld 验证旧密码错误时返回 BizError。
func TestAuthService_ChangePassword_WrongOld(t *testing.T) {
	svc, _, userRepo, _ := setupAuthTest(t)
	ctx := context.Background()

	user, err := userRepo.GetByUsername(ctx, "testuser")
	require.NoError(t, err)

	err = svc.ChangePassword(ctx, user.ID, "wrong-old-password", "new-password-123", "testuser", "127.0.0.1", "test-agent")
	require.Error(t, err)
	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeInvalidArgument, bizErr.Code)
}
