package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/middleware"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/service"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/stretchr/testify/require"
)

// testEnv 保存集成测试所需的共享组件。
type testEnv struct {
	router *gin.Engine
	jwtSvc *appcrypto.JWTService
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	// 1. 内存数据库
	db, err := database.Init(config.DatabaseConfig{
		Driver:       "sqlite",
		DSN:          ":memory:",
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	// 2. 迁移
	require.NoError(t, database.Migrate(db))

	// 3. 仓储
	userRepo := repository.NewUserRepository(db)
	mcpRepo := repository.NewMCPServiceRepository(db)
	toolRepo := repository.NewToolRepository(db)
	historyRepo := repository.NewRequestHistoryRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)

	// 4. JWT 服务
	jwtSvc := appcrypto.NewJWTService("test-secret-key-for-unit-test", "test", 2*time.Hour, 168*time.Hour, nil)

	// 5. 审计
	auditSink := service.NewDBAuditSink(auditRepo)

	// 6. 服务层
	authSvc := service.NewAuthService(userRepo, jwtSvc, auditSink)
	userSvc := service.NewUserService(userRepo, auditSink)
	auditSvc := service.NewAuditService(auditSink, auditRepo)

	manager := mcpclient.NewManager(config.AppConfig{Name: "test", Version: "0.1"})
	mcpSvc := service.NewMCPService(mcpRepo, manager, auditSink)
	toolSvc := service.NewToolService(toolRepo, mcpRepo, manager, auditSink)
	toolInvokeSvc := service.NewToolInvokeService(config.HistoryConfig{MaxBodyBytes: 8192}, toolRepo, mcpRepo, historyRepo, manager)

	// 7. 处理器
	authHandler := NewAuthHandler(authSvc)
	userHandler := NewUserHandler(userSvc, authSvc)
	_ = NewMCPHandler(mcpSvc)
	_ = NewToolHandler(toolSvc, toolInvokeSvc)
	_ = NewHistoryHandler(historyRepo)
	auditHandler := NewAuditHandler(auditSvc)

	// 8. 创建默认管理员
	hashed, err := appcrypto.HashPassword("admin123456")
	require.NoError(t, err)
	require.NoError(t, userRepo.Create(context.Background(), &entity.User{
		Username:     "root",
		Password:     hashed,
		Email:        "root@example.com",
		Role:         entity.RoleAdmin,
		IsActive:     true,
		IsFirstLogin: true,
	}))

	// 9. 路由（手动注册，避免循环引用 router 包）
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	api := r.Group("/api/v1")
	{
		api.POST("/auth/login", authHandler.Login)
		api.POST("/auth/refresh", authHandler.Refresh)
	}

	auth := api.Group("")
	auth.Use(middleware.Auth(jwtSvc))
	{
		auth.POST("/auth/logout", authHandler.Logout)
		auth.PUT("/users/:id/password", userHandler.ChangePassword)
	}

	modify := auth.Group("")
	modify.Use(middleware.RequireModify())
	{
		// MCP / Tool 路由（占位，本测试不直接使用）
	}

	admin := auth.Group("")
	admin.Use(middleware.RequireAdmin())
	{
		admin.GET("/users", userHandler.List)
		admin.POST("/users", userHandler.Create)
		admin.PUT("/users/:id", userHandler.Update)
		admin.DELETE("/users/:id", userHandler.Delete)
		admin.GET("/audit-logs", auditHandler.List)
	}

	return &testEnv{router: r, jwtSvc: jwtSvc}
}

// loginAsAdmin 使用默认管理员登录并返回 access_token。
func loginAsAdmin(t *testing.T, r *gin.Engine) string {
	t.Helper()
	body := `{"username":"root","password":"admin123456"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	return data["access_token"].(string)
}

// loginAndGetTokens 登录并返回 access_token 和 refresh_token。
func loginAndGetTokens(t *testing.T, r *gin.Engine) (string, string) {
	t.Helper()
	body := `{"username":"root","password":"admin123456"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	return data["access_token"].(string), data["refresh_token"].(string)
}

// authRequest 创建带认证头的请求。
func authRequest(method, url string, body string, token string) *http.Request {
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// ---------- Auth 测试 ----------

// TestAuthHandler_Login_Success 验证正确凭据登录返回 200 和 access_token。
func TestAuthHandler_Login_Success(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)
	require.NotEmpty(t, token)
}

// TestAuthHandler_Login_WrongPassword 验证错误密码返回 401。
func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	env := setupTestEnv(t)
	body := `{"username":"root","password":"wrongpass"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestAuthHandler_Login_MissingFields 验证缺少字段返回 400。
func TestAuthHandler_Login_MissingFields(t *testing.T) {
	env := setupTestEnv(t)
	body := `{"username":"root"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestAuthHandler_Refresh 验证刷新令牌返回 200 和新 access_token。
func TestAuthHandler_Refresh(t *testing.T) {
	env := setupTestEnv(t)
	_, refreshToken := loginAndGetTokens(t, env.router)

	body := `{"refresh_token":"` + refreshToken + `"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	require.NotEmpty(t, data["access_token"])
}

// TestAuthHandler_Logout 验证登出返回 200。
func TestAuthHandler_Logout(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	req := authRequest("POST", "/api/v1/auth/logout", `{}`, token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// ---------- User 测试 ----------

// TestUserHandler_Create 验证管理员可创建用户并返回 201。
func TestUserHandler_Create(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	body := `{"username":"newuser","password":"password123","email":"new@test.com","role":"readonly"}`
	req := authRequest("POST", "/api/v1/users", body, token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
}

// TestUserHandler_List 验证管理员可查询用户列表并返回 200。
func TestUserHandler_List(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	req := authRequest("GET", "/api/v1/users", "", token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	items := data["items"].([]any)
	require.GreaterOrEqual(t, len(items), 1, "至少应有 admin 用户")
}

// TestUserHandler_Create_Forbidden 验证只读用户无法创建用户并返回 403。
func TestUserHandler_Create_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	adminToken := loginAsAdmin(t, env.router)

	// 先用 admin 创建只读用户
	body := `{"username":"viewer","password":"password123","email":"viewer@test.com","role":"readonly"}`
	req := authRequest("POST", "/api/v1/users", body, adminToken)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// 用只读用户登录
	loginBody := `{"username":"viewer","password":"password123"}`
	req = httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var loginResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &loginResp))
	viewerToken := loginResp["data"].(map[string]any)["access_token"].(string)

	// 尝试创建用户，应返回 403
	body = `{"username":"another","password":"password123","email":"a@test.com","role":"readonly"}`
	req = authRequest("POST", "/api/v1/users", body, viewerToken)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

// TestUserHandler_ChangePassword 验证修改密码返回 200。
func TestUserHandler_ChangePassword(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	// 获取当前用户 ID
	claims, err := env.jwtSvc.ParseToken(token, appcrypto.TokenTypeAccess)
	require.NoError(t, err)

	body := `{"old_password":"admin123456","new_password":"newpass123456"}`
	req := authRequest("PUT", "/api/v1/users/"+claims.UserID+"/password", body, token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// ---------- Audit 测试 ----------

// TestAuditHandler_List 验证管理员可查询审计日志并返回 200。
func TestAuditHandler_List(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router) // 登录本身会产生审计记录

	req := authRequest("GET", "/api/v1/audit-logs", "", token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["code"])
}

