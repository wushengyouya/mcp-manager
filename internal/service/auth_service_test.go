package service

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/response"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAuthTest 初始化内存数据库、仓储、JWT 服务和 AuthService，并预置一个测试用户
func setupAuthTest(t *testing.T) (AuthService, *appcrypto.JWTService, repository.UserRepository, repository.AuthSessionRepository, *AuthStateManager, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth_test.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&entity.User{}, &entity.AuditLog{}, &entity.AuthSession{}))

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewAuthSessionRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	auditSink := NewDBAuditSink(auditRepo)

	jwtSvc := appcrypto.NewJWTService("test-secret-key-for-unit-tests", "test-issuer", 15*time.Minute, 24*time.Hour, nil)
	stateManager := NewAuthStateManager(userRepo, sessionRepo, NoopUserTokenVersionStore{}, NoopSessionStateStore{})
	jwtSvc.SetAccessTokenValidator(stateManager)

	// 预置测试用户
	hashed, err := appcrypto.HashPassword("correct-password")
	require.NoError(t, err)
	testUser := &entity.User{
		Username:     "testuser",
		Password:     hashed,
		Email:        "test@example.com",
		Role:         entity.RoleAdmin,
		IsActive:     true,
		TokenVersion: 1,
	}
	require.NoError(t, userRepo.Create(context.Background(), testUser))

	svc := NewAuthService(userRepo, sessionRepo, jwtSvc, auditSink, WithAuthStateManager(stateManager))
	return svc, jwtSvc, userRepo, sessionRepo, stateManager, db
}

// TestAuthService_Login 使用表驱动测试验证登录的各种场景
func TestAuthService_Login(t *testing.T) {
	svc, _, userRepo, _, stateManager, _ := setupAuthTest(t)
	ctx := context.Background()

	// 创建一个被禁用的用户：先创建再禁用，避免 GORM default:true 覆盖 false 零值
	hashed, err := appcrypto.HashPassword("disabled-pass")
	require.NoError(t, err)
	disabledUser := &entity.User{
		Username:     "disabled",
		Password:     hashed,
		Email:        "disabled@example.com",
		Role:         entity.RoleReadonly,
		IsActive:     true,
		TokenVersion: 1,
	}
	require.NoError(t, userRepo.Create(ctx, disabledUser))
	isActive := false
	_, err = NewUserService(userRepo, NoopAuditSink{}, WithUserAuthStateManager(stateManager)).Update(ctx, disabledUser.ID, UpdateUserInput{IsActive: &isActive}, AuditEntry{})
	require.NoError(t, err)

	tests := []struct {
		name     string
		username string
		password string
		wantCode int
		wantErr  bool
	}{
		{name: "成功登录", username: "testuser", password: "correct-password"},
		{name: "密码错误", username: "testuser", password: "wrong-password", wantCode: response.CodeUnauthorized, wantErr: true},
		{name: "用户不存在", username: "nonexistent", password: "any-password", wantCode: response.CodeUnauthorized, wantErr: true},
		{name: "用户已禁用", username: "disabled", password: "disabled-pass", wantCode: response.CodeForbidden, wantErr: true},
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
				return
			}
			require.NoError(t, err)
			require.NotNil(t, pair)
			require.NotEmpty(t, pair.AccessToken)
			require.NotEmpty(t, pair.RefreshToken)
			require.NotNil(t, user)
			require.Equal(t, tt.username, user.Username)
			require.NotNil(t, user.LastLoginAt)
		})
	}
}

// TestAuthService_Logout 验证登出后 access token 被拉黑、refresh session 被撤销。
func TestAuthService_Logout(t *testing.T) {
	svc, jwtSvc, _, sessionRepo, _, _ := setupAuthTest(t)
	ctx := context.Background()

	pair, _, err := svc.Login(ctx, "testuser", "correct-password", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	claims, err := jwtSvc.ParseAccessToken(ctx, pair.AccessToken)
	require.NoError(t, err)
	err = svc.Logout(ctx, pair.AccessToken, pair.RefreshToken, "user-id", "testuser", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	_, err = jwtSvc.ParseAccessToken(ctx, pair.AccessToken)
	require.Error(t, err)

	session, err := sessionRepo.GetByID(ctx, claims.SessionID)
	require.NoError(t, err)
	require.Equal(t, entity.AuthSessionStatusRevoked, session.Status)

	_, err = svc.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err)
}

// TestAuthService_Refresh 验证 refresh token 轮换返回新的令牌对。
func TestAuthService_Refresh(t *testing.T) {
	svc, jwtSvc, _, sessionRepo, _, _ := setupAuthTest(t)
	ctx := context.Background()

	pair, _, err := svc.Login(ctx, "testuser", "correct-password", "127.0.0.1", "test-agent")
	require.NoError(t, err)
	oldClaims, err := jwtSvc.ParseAccessToken(ctx, pair.AccessToken)
	require.NoError(t, err)

	newPair, err := svc.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)
	require.NotNil(t, newPair)
	require.NotEmpty(t, newPair.AccessToken)
	require.NotEmpty(t, newPair.RefreshToken)
	require.NotEqual(t, pair.RefreshToken, newPair.RefreshToken)

	_, err = jwtSvc.ParseAccessToken(ctx, newPair.AccessToken)
	require.NoError(t, err)

	oldSession, err := sessionRepo.GetByID(ctx, oldClaims.SessionID)
	require.NoError(t, err)
	require.Equal(t, entity.AuthSessionStatusRotated, oldSession.Status)

	_, err = svc.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err)
}

// TestAuthService_Refresh_ConcurrentSingleUse 验证并发 refresh 只有一个请求能成功。
func TestAuthService_Refresh_ConcurrentSingleUse(t *testing.T) {
	svc, _, _, _, _, _ := setupAuthTest(t)
	ctx := context.Background()

	pair, _, err := svc.Login(ctx, "testuser", "correct-password", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	const workers = 2
	results := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err = svc.Refresh(ctx, pair.RefreshToken)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, failures)
}

// TestAuthService_Refresh_ReuseOnlyBurnsOnce 验证同一个 rotated refresh token 只会触发一次全量撤销。
func TestAuthService_Refresh_ReuseOnlyBurnsOnce(t *testing.T) {
	svc, jwtSvc, _, _, _, _ := setupAuthTest(t)
	ctx := context.Background()

	pair, _, err := svc.Login(ctx, "testuser", "correct-password", "127.0.0.1", "test-agent")
	require.NoError(t, err)
	_, err = svc.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)

	_, err = svc.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err)

	afterBurnPair, _, err := svc.Login(ctx, "testuser", "correct-password", "127.0.0.1", "test-agent")
	require.NoError(t, err)
	_, err = jwtSvc.ParseAccessToken(ctx, afterBurnPair.AccessToken)
	require.NoError(t, err)

	_, err = svc.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err)
	_, err = jwtSvc.ParseAccessToken(ctx, afterBurnPair.AccessToken)
	require.NoError(t, err)

}

// TestAuthService_Refresh_InvalidToken 验证无效 refresh token 返回错误。
func TestAuthService_Refresh_InvalidToken(t *testing.T) {
	svc, jwtSvc, _, _, _, _ := setupAuthTest(t)
	ctx := context.Background()

	legacyRefresh, _, err := jwtSvc.GenerateAccessToken("u-legacy", "legacy-session", "legacy", string(entity.RoleAdmin), 1)
	require.NoError(t, err)

	for _, token := range []string{"invalid-token-string", legacyRefresh} {
		_, err := svc.Refresh(ctx, token)
		require.Error(t, err)
		var bizErr *response.BizError
		require.ErrorAs(t, err, &bizErr)
		require.Equal(t, response.CodeUnauthorized, bizErr.Code)
	}
}

// TestJWTService_SharedRedisBlacklist 验证 Redis 黑名单可在多个 JWT 实例间共享。
func TestJWTService_SharedRedisBlacklist(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	storeA := appcrypto.NewRedisTokenBlacklistStore(client, appcrypto.RedisBlacklistOptions{KeyPrefix: "auth-test:", OperationTimeout: time.Second})
	storeB := appcrypto.NewRedisTokenBlacklistStore(client, appcrypto.RedisBlacklistOptions{KeyPrefix: "auth-test:", OperationTimeout: time.Second})

	jwtA := appcrypto.NewJWTService("shared-secret", "issuer", time.Hour, 2*time.Hour, storeA)
	jwtB := appcrypto.NewJWTService("shared-secret", "issuer", time.Hour, 2*time.Hour, storeB)

	accessToken, expireAt, err := jwtA.GenerateAccessToken("u-1", "session-1", "tester", string(entity.RoleAdmin), 1)
	require.NoError(t, err)

	jwtA.Blacklist(accessToken, expireAt)
	_, err = jwtB.ParseAccessToken(context.Background(), accessToken)
	require.Error(t, err)
}

// TestAuthService_ChangePassword_Success 验证成功修改密码后旧 refresh token 失效。
func TestAuthService_ChangePassword_Success(t *testing.T) {
	svc, _, userRepo, _, _, _ := setupAuthTest(t)
	ctx := context.Background()

	user, err := userRepo.GetByUsername(ctx, "testuser")
	require.NoError(t, err)
	pair, _, err := svc.Login(ctx, "testuser", "correct-password", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	err = svc.ChangePassword(ctx, user.ID, "correct-password", "new-password-123", "testuser", "127.0.0.1", "test-agent")
	require.NoError(t, err)

	_, _, err = svc.Login(ctx, "testuser", "correct-password", "127.0.0.1", "test-agent")
	require.Error(t, err)

	pair2, _, err := svc.Login(ctx, "testuser", "new-password-123", "127.0.0.1", "test-agent")
	require.NoError(t, err)
	require.NotNil(t, pair2)

	_, err = svc.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err)
}

// TestAuthService_ChangePassword_WrongOld 验证旧密码错误时返回 BizError
func TestAuthService_ChangePassword_WrongOld(t *testing.T) {
	svc, _, userRepo, _, _, _ := setupAuthTest(t)
	ctx := context.Background()

	user, err := userRepo.GetByUsername(ctx, "testuser")
	require.NoError(t, err)

	err = svc.ChangePassword(ctx, user.ID, "wrong-old-password", "new-password-123", "testuser", "127.0.0.1", "test-agent")
	require.Error(t, err)
	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeInvalidArgument, bizErr.Code)
}
