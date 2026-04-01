package handler

import (
	"context"
	"encoding/json"
	"fmt"
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

// testEnv 保存集成测试所需的共享组件
type testEnv struct {
	router      *gin.Engine
	jwtSvc      *appcrypto.JWTService
	userRepo    repository.UserRepository
	mcpRepo     repository.MCPServiceRepository
	toolRepo    repository.ToolRepository
	historyRepo repository.RequestHistoryRepository
	auditRepo   repository.AuditLogRepository
}

// setupTestEnv 初始化处理器测试所需的内存依赖
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
	mcpSvc := service.NewMCPService(mcpRepo, toolRepo, manager, auditSink, nil)
	toolSvc := service.NewToolService(toolRepo, mcpRepo, manager, auditSink)
	toolInvokeSvc := service.NewToolInvokeService(config.HistoryConfig{MaxBodyBytes: 8192}, toolRepo, mcpRepo, historyRepo, manager)

	// 7. 处理器
	authHandler := NewAuthHandler(authSvc)
	userHandler := NewUserHandler(userSvc, authSvc)
	mcpHandler := NewMCPHandler(mcpSvc)
	toolHandler := NewToolHandler(toolSvc, toolInvokeSvc)
	historyHandler := NewHistoryHandler(historyRepo)
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
		auth.GET("/services", mcpHandler.List)
		auth.GET("/services/:id", mcpHandler.Get)
		auth.GET("/services/:id/status", mcpHandler.Status)
		auth.GET("/services/:id/tools", toolHandler.ListByService)
		auth.GET("/tools/:id", toolHandler.Get)
		auth.GET("/history", historyHandler.List)
		auth.GET("/history/:id", historyHandler.Get)
		auth.PUT("/users/:id/password", userHandler.ChangePassword)
	}

	modify := auth.Group("")
	modify.Use(middleware.RequireModify())
	{
		modify.POST("/services", mcpHandler.Create)
		modify.PUT("/services/:id", mcpHandler.Update)
		modify.DELETE("/services/:id", mcpHandler.Delete)
		modify.POST("/services/:id/disconnect", mcpHandler.Disconnect)
	}

	admin := auth.Group("")
	admin.Use(middleware.RequireAdmin())
	{
		admin.GET("/users", userHandler.List)
		admin.POST("/users", userHandler.Create)
		admin.PUT("/users/:id", userHandler.Update)
		admin.DELETE("/users/:id", userHandler.Delete)
		admin.GET("/audit-logs", auditHandler.List)
		admin.GET("/audit-logs/export", auditHandler.Export)
	}

	return &testEnv{
		router:      r,
		jwtSvc:      jwtSvc,
		userRepo:    userRepo,
		mcpRepo:     mcpRepo,
		toolRepo:    toolRepo,
		historyRepo: historyRepo,
		auditRepo:   auditRepo,
	}
}

// loginAsAdmin 使用默认管理员登录并返回 access_token
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

// loginAndGetTokens 登录并返回 access_token 和 refresh_token
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

// authRequest 创建带认证头的请求
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

// TestAuthHandler_Login_Success 验证正确凭据登录返回 200 和 access_token
func TestAuthHandler_Login_Success(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)
	require.NotEmpty(t, token)
}

// TestAuthHandler_Login_WrongPassword 验证错误密码返回 401
func TestAuthHandler_Login_WrongPassword(t *testing.T) {
	env := setupTestEnv(t)
	body := `{"username":"root","password":"wrongpass"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestAuthHandler_Login_MissingFields 验证缺少字段返回 400
func TestAuthHandler_Login_MissingFields(t *testing.T) {
	env := setupTestEnv(t)
	body := `{"username":"root"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestAuthHandler_Refresh 验证刷新令牌返回 200 和新 access_token
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

// TestAuthHandler_Logout 验证登出返回 200
func TestAuthHandler_Logout(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	req := authRequest("POST", "/api/v1/auth/logout", `{}`, token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_Logout_BadJSON(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	req := authRequest("POST", "/api/v1/auth/logout", `{"refresh_token":`, token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandler_Logout_BodyTooLarge(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	body := fmt.Sprintf(`{"refresh_token":"%s"}`, strings.Repeat("a", optionalJSONBodyLimit))
	req := authRequest("POST", "/api/v1/auth/logout", body, token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

// ---------- User 测试 ----------

// TestUserHandler_Create 验证管理员可创建用户并返回 201
func TestUserHandler_Create(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	body := `{"username":"newuser","password":"password123","email":"new@test.com","role":"readonly"}`
	req := authRequest("POST", "/api/v1/users", body, token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
}

// TestUserHandler_List 验证管理员可查询用户列表并返回 200
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

func TestUserHandler_List_InvalidQuery(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	for _, rawURL := range []string{
		"/api/v1/users?page=abc",
		"/api/v1/users?page_size=0",
		"/api/v1/users?page_size=101",
		"/api/v1/users?role=bad",
		"/api/v1/users?active=abc",
	} {
		req := authRequest("GET", rawURL, "", token)
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code, rawURL)
	}
}

func TestUserHandler_List_EmptyOptionalQueryUsesDefaults(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	hashed, err := appcrypto.HashPassword("password123")
	require.NoError(t, err)
	require.NoError(t, env.userRepo.Create(context.Background(), &entity.User{
		Username:     "operator-empty-query",
		Password:     hashed,
		Email:        "operator-empty-query@example.com",
		Role:         entity.RoleOperator,
		IsActive:     true,
		IsFirstLogin: true,
	}))
	require.NoError(t, env.userRepo.Create(context.Background(), &entity.User{
		Username:     "readonly-inactive",
		Password:     hashed,
		Email:        "readonly-inactive@example.com",
		Role:         entity.RoleReadonly,
		IsActive:     false,
		IsFirstLogin: true,
	}))

	t.Run("active empty means unset", func(t *testing.T) {
		req := authRequest("GET", "/api/v1/users?active=", "", token)
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].(map[string]any)
		items := data["items"].([]any)
		require.Len(t, items, 3)
		require.EqualValues(t, 3, data["total"])
	})

	t.Run("empty paging falls back to defaults", func(t *testing.T) {
		req := authRequest("GET", "/api/v1/users?page=&page_size=", "", token)
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].(map[string]any)
		require.EqualValues(t, 1, data["page"])
		require.EqualValues(t, 10, data["page_size"])
		require.EqualValues(t, 3, data["total"])
	})

	t.Run("mixed empty and non-empty keeps non-empty value", func(t *testing.T) {
		req := authRequest("GET", "/api/v1/users?page=&page=2", "", token)
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data := resp["data"].(map[string]any)
		require.EqualValues(t, 2, data["page"])
		require.EqualValues(t, 10, data["page_size"])
	})
}

// TestUserHandler_Create_Forbidden 验证只读用户无法创建用户并返回 403
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

// TestUserHandler_ChangePassword 验证修改密码返回 200
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

// TestAuditHandler_List 验证管理员可查询审计日志并返回 200
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

func TestAuditHandler_List_InvalidQuery(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	req := authRequest("GET", "/api/v1/audit-logs?page=abc", "", token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserHandler_Update(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)
	hashed, err := appcrypto.HashPassword("password123")
	require.NoError(t, err)
	target := &entity.User{
		Username:     "to-update",
		Password:     hashed,
		Email:        "update-old@example.com",
		Role:         entity.RoleReadonly,
		IsActive:     true,
		IsFirstLogin: true,
	}
	require.NoError(t, env.userRepo.Create(context.Background(), target))

	body := `{"email":"update-new@example.com","role":"operator","is_active":false}`
	req := authRequest("PUT", "/api/v1/users/"+target.ID, body, token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	got, err := env.userRepo.GetByID(context.Background(), target.ID)
	require.NoError(t, err)
	require.Equal(t, "update-new@example.com", got.Email)
	require.Equal(t, entity.RoleOperator, got.Role)
	require.False(t, got.IsActive)
}

func TestUserHandler_Update_EmptyBody(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)
	hashed, err := appcrypto.HashPassword("password123")
	require.NoError(t, err)
	target := &entity.User{
		Username:     "to-update-empty",
		Password:     hashed,
		Email:        "empty-update@example.com",
		Role:         entity.RoleReadonly,
		IsActive:     true,
		IsFirstLogin: true,
	}
	require.NoError(t, env.userRepo.Create(context.Background(), target))

	req := authRequest("PUT", "/api/v1/users/"+target.ID, `{}`, token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUserHandler_Delete(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)
	hashed, err := appcrypto.HashPassword("password123")
	require.NoError(t, err)
	target := &entity.User{
		Username:     "to-delete",
		Password:     hashed,
		Email:        "delete@example.com",
		Role:         entity.RoleReadonly,
		IsActive:     true,
		IsFirstLogin: true,
	}
	require.NoError(t, env.userRepo.Create(context.Background(), target))

	req := authRequest("DELETE", "/api/v1/users/"+target.ID, "", token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	_, err = env.userRepo.GetByID(context.Background(), target.ID)
	require.ErrorIs(t, err, repository.ErrNotFound)
}

func TestAuditHandler_Export(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	req := authRequest("GET", "/api/v1/audit-logs/export?action=login", "", token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "attachment; filename=audit_logs.csv", w.Header().Get("Content-Disposition"))
	require.Contains(t, w.Body.String(), "username,action,resource_type")
}

func TestHistoryHandler_ListAndGet(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)
	claims, err := env.jwtSvc.ParseToken(token, appcrypto.TokenTypeAccess)
	require.NoError(t, err)

	service := &entity.MCPService{
		Name:          "history-service",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://history.test/mcp",
	}
	require.NoError(t, env.mcpRepo.Create(context.Background(), service))

	item := &entity.RequestHistory{
		ID:              "history-1",
		MCPServiceID:    service.ID,
		ToolName:        "search",
		UserID:          claims.UserID,
		RequestBody:     entity.JSONMap{"q": "hello"},
		ResponseBody:    entity.JSONMap{"ok": true},
		CompressionType: "none",
		Status:          entity.RequestStatusSuccess,
		DurationMS:      5,
		CreatedAt:       time.Now().UTC(),
	}
	require.NoError(t, env.historyRepo.Create(context.Background(), item))

	req := authRequest("GET", "/api/v1/history?service_id="+service.ID+"&tool_name=search&status=success", "", token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = authRequest("GET", "/api/v1/history/"+item.ID, "", token)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestHistoryHandler_List_InvalidTime(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	for _, rawURL := range []string{
		"/api/v1/history?start_at=bad-time",
		"/api/v1/history?end_at=bad-time",
	} {
		req := authRequest("GET", rawURL, "", token)
		w := httptest.NewRecorder()
		env.router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code, rawURL)
	}
}

func TestHistoryHandler_List_EmptyPagingQueryUsesDefaults(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	req := authRequest("GET", "/api/v1/history?page=&page_size=", "", token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]any)
	require.EqualValues(t, 1, data["page"])
	require.EqualValues(t, 10, data["page_size"])
}

func TestHistoryHandler_Get_Forbidden(t *testing.T) {
	env := setupTestEnv(t)
	adminToken := loginAsAdmin(t, env.router)
	adminClaims, err := env.jwtSvc.ParseToken(adminToken, appcrypto.TokenTypeAccess)
	require.NoError(t, err)

	hashed, err := appcrypto.HashPassword("password123")
	require.NoError(t, err)
	require.NoError(t, env.userRepo.Create(context.Background(), &entity.User{
		Username:     "viewer",
		Password:     hashed,
		Email:        "viewer@example.com",
		Role:         entity.RoleReadonly,
		IsActive:     true,
		IsFirstLogin: true,
	}))

	service := &entity.MCPService{
		Name:          "forbidden-history-service",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://history.test/mcp",
	}
	require.NoError(t, env.mcpRepo.Create(context.Background(), service))

	require.NoError(t, env.historyRepo.Create(context.Background(), &entity.RequestHistory{
		ID:              "history-forbidden",
		MCPServiceID:    service.ID,
		ToolName:        "search",
		UserID:          adminClaims.UserID,
		RequestBody:     entity.JSONMap{"q": "hello"},
		ResponseBody:    entity.JSONMap{"ok": true},
		CompressionType: "none",
		Status:          entity.RequestStatusSuccess,
		DurationMS:      5,
		CreatedAt:       time.Now().UTC(),
	}))

	loginBody := `{"username":"viewer","password":"password123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var loginResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &loginResp))
	viewerToken := loginResp["data"].(map[string]any)["access_token"].(string)

	req = authRequest("GET", "/api/v1/history/history-forbidden", "", viewerToken)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestMCPHandler_CRUDStatusDisconnectAndToolViews(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAsAdmin(t, env.router)

	createBody := `{"name":"svc-http","transport_type":"streamable_http","url":"http://svc-http.test/mcp","tags":["alpha","team-a"]}`
	req := authRequest("POST", "/api/v1/services", createBody, token)
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var createResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	serviceData := createResp["data"].(map[string]any)
	serviceID := serviceData["id"].(string)

	req = authRequest("GET", "/api/v1/services?transport_type=streamable_http&tag=alpha", "", token)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = authRequest("GET", "/api/v1/services?transport_type=bad", "", token)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	req = authRequest("GET", "/api/v1/services/"+serviceID, "", token)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = authRequest("GET", "/api/v1/services/"+serviceID+"/status", "", token)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	updateBody := `{"name":"svc-http","transport_type":"streamable_http","url":"http://svc-http.test/v2","tags":["beta"]}`
	req = authRequest("PUT", "/api/v1/services/"+serviceID, updateBody, token)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	require.NoError(t, env.toolRepo.Create(context.Background(), &entity.Tool{
		MCPServiceID: serviceID,
		Name:         "search",
		Description:  "search tool",
		IsEnabled:    true,
		SyncedAt:     time.Now().UTC(),
	}))

	req = authRequest("GET", "/api/v1/services/"+serviceID+"/tools", "", token)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	tools, err := env.toolRepo.ListByService(context.Background(), serviceID)
	require.NoError(t, err)
	require.Len(t, tools, 1)

	req = authRequest("GET", "/api/v1/tools/"+tools[0].ID, "", token)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = authRequest("POST", "/api/v1/services/"+serviceID+"/disconnect", "", token)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = authRequest("DELETE", "/api/v1/services/"+serviceID, "", token)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
