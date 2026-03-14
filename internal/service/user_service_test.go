package service

import (
	"context"
	"testing"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupUserTest 初始化内存数据库、仓储和 UserService。
func setupUserTest(t *testing.T) (UserService, repository.UserRepository, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&entity.User{}, &entity.AuditLog{}))

	userRepo := repository.NewUserRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	auditSink := NewDBAuditSink(auditRepo)

	svc := NewUserService(userRepo, auditSink)
	return svc, userRepo, db
}

func testActor() AuditEntry {
	return AuditEntry{UserID: "admin-id", Username: "admin"}
}

// TestUserService_Create_Success 验证成功创建用户。
func TestUserService_Create_Success(t *testing.T) {
	svc, _, _ := setupUserTest(t)
	ctx := context.Background()

	user, err := svc.Create(ctx, CreateUserInput{
		Username: "newuser",
		Password: "password123",
		Email:    "new@example.com",
		Role:     entity.RoleOperator,
	}, testActor())

	require.NoError(t, err)
	require.NotNil(t, user)
	require.Equal(t, "newuser", user.Username)
	require.Equal(t, "new@example.com", user.Email)
	require.Equal(t, entity.RoleOperator, user.Role)
	require.True(t, user.IsActive)
	require.True(t, user.IsFirstLogin)
	require.NotEmpty(t, user.ID)
	// 密码应该是哈希后的，不等于明文
	require.NotEqual(t, "password123", user.Password)
}

// TestUserService_Create_DuplicateUsername 验证用户名重复返回 CodeConflict。
func TestUserService_Create_DuplicateUsername(t *testing.T) {
	svc, _, _ := setupUserTest(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateUserInput{
		Username: "dupuser",
		Password: "password123",
		Email:    "dup1@example.com",
		Role:     entity.RoleReadonly,
	}, testActor())
	require.NoError(t, err)

	_, err = svc.Create(ctx, CreateUserInput{
		Username: "dupuser",
		Password: "password456",
		Email:    "dup2@example.com",
		Role:     entity.RoleReadonly,
	}, testActor())
	require.Error(t, err)

	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeConflict, bizErr.Code)
}

// TestUserService_Create_DuplicateEmail 验证邮箱重复返回 CodeConflict。
func TestUserService_Create_DuplicateEmail(t *testing.T) {
	svc, _, _ := setupUserTest(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateUserInput{
		Username: "user1",
		Password: "password123",
		Email:    "same@example.com",
		Role:     entity.RoleReadonly,
	}, testActor())
	require.NoError(t, err)

	_, err = svc.Create(ctx, CreateUserInput{
		Username: "user2",
		Password: "password456",
		Email:    "same@example.com",
		Role:     entity.RoleReadonly,
	}, testActor())
	require.Error(t, err)

	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeConflict, bizErr.Code)
}

// TestUserService_Create_InvalidPassword 验证密码过短被拒绝。
func TestUserService_Create_InvalidPassword(t *testing.T) {
	svc, _, _ := setupUserTest(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateUserInput{
		Username: "shortpw",
		Password: "12345", // 少于 6 字节
		Email:    "short@example.com",
		Role:     entity.RoleReadonly,
	}, testActor())
	require.Error(t, err)

	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeInvalidArgument, bizErr.Code)
}

// TestUserService_Update_Success 验证成功更新用户邮箱。
func TestUserService_Update_Success(t *testing.T) {
	svc, _, _ := setupUserTest(t)
	ctx := context.Background()

	user, err := svc.Create(ctx, CreateUserInput{
		Username: "updateme",
		Password: "password123",
		Email:    "old@example.com",
		Role:     entity.RoleReadonly,
	}, testActor())
	require.NoError(t, err)

	updated, err := svc.Update(ctx, user.ID, UpdateUserInput{
		Email: "new@example.com",
		Role:  entity.RoleOperator,
	}, testActor())
	require.NoError(t, err)
	require.Equal(t, "new@example.com", updated.Email)
	require.Equal(t, entity.RoleOperator, updated.Role)
}

// TestUserService_Update_DuplicateEmail 验证更新为已存在的邮箱返回 CodeConflict。
func TestUserService_Update_DuplicateEmail(t *testing.T) {
	svc, _, _ := setupUserTest(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, CreateUserInput{
		Username: "user_a",
		Password: "password123",
		Email:    "taken@example.com",
		Role:     entity.RoleReadonly,
	}, testActor())
	require.NoError(t, err)

	userB, err := svc.Create(ctx, CreateUserInput{
		Username: "user_b",
		Password: "password456",
		Email:    "free@example.com",
		Role:     entity.RoleReadonly,
	}, testActor())
	require.NoError(t, err)

	_, err = svc.Update(ctx, userB.ID, UpdateUserInput{
		Email: "taken@example.com",
	}, testActor())
	require.Error(t, err)

	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeConflict, bizErr.Code)
}

// TestUserService_Delete_Success 验证成功删除用户。
func TestUserService_Delete_Success(t *testing.T) {
	svc, _, _ := setupUserTest(t)
	ctx := context.Background()

	user, err := svc.Create(ctx, CreateUserInput{
		Username: "deleteme",
		Password: "password123",
		Email:    "delete@example.com",
		Role:     entity.RoleReadonly,
	}, testActor())
	require.NoError(t, err)

	err = svc.Delete(ctx, user.ID, "admin-id", testActor())
	require.NoError(t, err)

	// 删除后应该无法获取
	_, err = svc.Get(ctx, user.ID)
	require.Error(t, err)
}

// TestUserService_Delete_Self 验证不能删除自己。
func TestUserService_Delete_Self(t *testing.T) {
	svc, _, _ := setupUserTest(t)
	ctx := context.Background()

	user, err := svc.Create(ctx, CreateUserInput{
		Username: "selfdelete",
		Password: "password123",
		Email:    "self@example.com",
		Role:     entity.RoleAdmin,
	}, testActor())
	require.NoError(t, err)

	err = svc.Delete(ctx, user.ID, user.ID, testActor())
	require.Error(t, err)

	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeInvalidArgument, bizErr.Code)
	require.Contains(t, bizErr.Message, "不能删除自己")
}

// TestUserService_Get_NotFound 验证获取不存在的用户返回错误。
func TestUserService_Get_NotFound(t *testing.T) {
	svc, _, _ := setupUserTest(t)
	ctx := context.Background()

	_, err := svc.Get(ctx, "nonexistent-id")
	require.Error(t, err)
}

// TestUserService_List 验证用户列表分页和过滤。
func TestUserService_List(t *testing.T) {
	svc, _, _ := setupUserTest(t)
	ctx := context.Background()

	// 创建多个用户
	for i, name := range []string{"alice", "bob", "charlie"} {
		roles := []entity.Role{entity.RoleAdmin, entity.RoleOperator, entity.RoleReadonly}
		_, err := svc.Create(ctx, CreateUserInput{
			Username: name,
			Password: "password123",
			Email:    name + "@example.com",
			Role:     roles[i],
		}, testActor())
		require.NoError(t, err)
	}

	// 列出所有用户
	users, total, err := svc.List(ctx, repository.UserListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, users, 3)

	// 按角色过滤
	users, total, err = svc.List(ctx, repository.UserListFilter{Page: 1, PageSize: 10, Role: "admin"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, users, 1)
	require.Equal(t, "alice", users[0].Username)

	// 分页
	users, total, err = svc.List(ctx, repository.UserListFilter{Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, users, 2)
}
