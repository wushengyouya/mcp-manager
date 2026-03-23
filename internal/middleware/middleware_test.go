package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/response"
	"github.com/stretchr/testify/require"
)

// init 初始化 Gin 测试模式
func init() {
	gin.SetMode(gin.TestMode)
}

// respBody 用于解析统一响应 JSON
type respBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// newJWTService 创建测试用 JWTService
func newJWTService() *appcrypto.JWTService {
	return appcrypto.NewJWTService("test-secret", "test-issuer", time.Hour, 2*time.Hour, appcrypto.NewTokenBlacklist())
}

// ---------- Auth 中间件测试 ----------

// TestAuth_TableDriven 使用表驱动方式测试 Auth 中间件的各种场景
func TestAuth_TableDriven(t *testing.T) {
	jwtSvc := newJWTService()

	// 预生成合法 access token
	pair, err := jwtSvc.GenerateTokenPair("user-1", "alice", string(entity.RoleAdmin))
	require.NoError(t, err)
	validAccessToken := pair.AccessToken
	validRefreshToken := pair.RefreshToken

	// 将一个合法 token 加入黑名单
	blacklistedPair, err := jwtSvc.GenerateTokenPair("user-2", "bob", string(entity.RoleOperator))
	require.NoError(t, err)
	blacklistedToken := blacklistedPair.AccessToken
	jwtSvc.Blacklist(blacklistedToken, time.Now().Add(time.Hour))

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
		wantCode   int
		wantPass   bool // 是否期望请求通过中间件到达 handler
	}{
		{
			name:       "有效 access token 通过认证",
			authHeader: "Bearer " + validAccessToken,
			wantStatus: http.StatusOK,
			wantCode:   response.CodeSuccess,
			wantPass:   true,
		},
		{
			name:       "无 Authorization 头返回 401",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantCode:   response.CodeUnauthorized,
			wantPass:   false,
		},
		{
			name:       "缺少 Bearer 前缀返回 401",
			authHeader: "Token " + validAccessToken,
			wantStatus: http.StatusUnauthorized,
			wantCode:   response.CodeUnauthorized,
			wantPass:   false,
		},
		{
			name:       "无效/畸形 token 返回 401",
			authHeader: "Bearer this.is.not.a.valid.jwt",
			wantStatus: http.StatusUnauthorized,
			wantCode:   response.CodeUnauthorized,
			wantPass:   false,
		},
		{
			name:       "黑名单 token 返回 401",
			authHeader: "Bearer " + blacklistedToken,
			wantStatus: http.StatusUnauthorized,
			wantCode:   response.CodeUnauthorized,
			wantPass:   false,
		},
		{
			name:       "refresh token 类型不匹配返回 401",
			authHeader: "Bearer " + validRefreshToken,
			wantStatus: http.StatusUnauthorized,
			wantCode:   response.CodeUnauthorized,
			wantPass:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, r := gin.CreateTestContext(w)

			passed := false
			r.Use(Auth(jwtSvc))
			r.GET("/test", func(c *gin.Context) {
				passed = true
				response.Success(c, nil)
			})

			c.Request, _ = http.NewRequest(http.MethodGet, "/test", nil)
			if tt.authHeader != "" {
				c.Request.Header.Set("Authorization", tt.authHeader)
			}
			r.ServeHTTP(w, c.Request)

			require.Equal(t, tt.wantStatus, w.Code)

			var body respBody
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, tt.wantCode, body.Code)
			require.Equal(t, tt.wantPass, passed)
		})
	}
}

// TestAuth_ValidToken_SetsContext 验证有效 token 是否正确设置用户上下文
func TestAuth_ValidToken_SetsContext(t *testing.T) {
	jwtSvc := newJWTService()

	pair, err := jwtSvc.GenerateTokenPair("uid-42", "charlie", string(entity.RoleOperator))
	require.NoError(t, err)

	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)

	var gotUserID, gotUsername string
	var gotRole entity.Role

	r.Use(Auth(jwtSvc))
	r.GET("/me", func(c *gin.Context) {
		gotUserID, gotUsername, gotRole = CurrentUser(c)
		response.Success(c, nil)
	})

	req, _ := http.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "uid-42", gotUserID)
	require.Equal(t, "charlie", gotUsername)
	require.Equal(t, entity.RoleOperator, gotRole)
}

// TestAuth_ExpiredToken 验证过期 token 返回 CodeTokenExpired
func TestAuth_ExpiredToken(t *testing.T) {
	// 创建一个 TTL 极短的 JWTService
	shortSvc := appcrypto.NewJWTService("test-secret", "test-issuer", time.Millisecond, 2*time.Hour, appcrypto.NewTokenBlacklist())
	pair, err := shortSvc.GenerateTokenPair("uid-exp", "expired-user", string(entity.RoleReadonly))
	require.NoError(t, err)

	// 等待 token 过期
	time.Sleep(50 * time.Millisecond)

	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)

	r.Use(Auth(shortSvc))
	r.GET("/test", func(c *gin.Context) {
		response.Success(c, nil)
	})

	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)

	var body respBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, response.CodeTokenExpired, body.Code)
}

// ---------- Permission 权限中间件测试 ----------

// setAuthContext 模拟已认证用户，将用户信息写入 gin.Context
func setAuthContext(c *gin.Context, userID, username string, role entity.Role) {
	c.Set("current_user_id", userID)
	c.Set("current_username", username)
	c.Set("current_role", string(role))
}

// TestRequireAdmin_TableDriven 测试 RequireAdmin 中间件对各角色的处理
func TestRequireAdmin_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		role       entity.Role
		wantStatus int
		wantCode   int
		wantPass   bool
	}{
		{
			name:       "admin 角色通过",
			role:       entity.RoleAdmin,
			wantStatus: http.StatusOK,
			wantCode:   response.CodeSuccess,
			wantPass:   true,
		},
		{
			name:       "operator 角色被拒绝",
			role:       entity.RoleOperator,
			wantStatus: http.StatusForbidden,
			wantCode:   response.CodeForbidden,
			wantPass:   false,
		},
		{
			name:       "readonly 角色被拒绝",
			role:       entity.RoleReadonly,
			wantStatus: http.StatusForbidden,
			wantCode:   response.CodeForbidden,
			wantPass:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			_, r := gin.CreateTestContext(w)

			passed := false
			r.Use(func(c *gin.Context) {
				setAuthContext(c, "u1", "testuser", tt.role)
				c.Next()
			})
			r.Use(RequireAdmin())
			r.GET("/admin", func(c *gin.Context) {
				passed = true
				response.Success(c, nil)
			})

			req, _ := http.NewRequest(http.MethodGet, "/admin", nil)
			r.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)

			var body respBody
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, tt.wantCode, body.Code)
			require.Equal(t, tt.wantPass, passed)
		})
	}
}

// TestRequireModify_TableDriven 测试 RequireModify 中间件对各角色的处理
func TestRequireModify_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		role       entity.Role
		wantStatus int
		wantCode   int
		wantPass   bool
	}{
		{
			name:       "admin 角色通过",
			role:       entity.RoleAdmin,
			wantStatus: http.StatusOK,
			wantCode:   response.CodeSuccess,
			wantPass:   true,
		},
		{
			name:       "operator 角色通过",
			role:       entity.RoleOperator,
			wantStatus: http.StatusOK,
			wantCode:   response.CodeSuccess,
			wantPass:   true,
		},
		{
			name:       "readonly 角色被拒绝",
			role:       entity.RoleReadonly,
			wantStatus: http.StatusForbidden,
			wantCode:   response.CodeForbidden,
			wantPass:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			_, r := gin.CreateTestContext(w)

			passed := false
			r.Use(func(c *gin.Context) {
				setAuthContext(c, "u1", "testuser", tt.role)
				c.Next()
			})
			r.Use(RequireModify())
			r.GET("/modify", func(c *gin.Context) {
				passed = true
				response.Success(c, nil)
			})

			req, _ := http.NewRequest(http.MethodGet, "/modify", nil)
			r.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)

			var body respBody
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, tt.wantCode, body.Code)
			require.Equal(t, tt.wantPass, passed)
		})
	}
}

// TestRequireRole_CustomRoles 测试 RequireRole 接受自定义角色列表
func TestRequireRole_CustomRoles(t *testing.T) {
	customRole := entity.Role("auditor")

	tests := []struct {
		name       string
		allowed    []entity.Role
		userRole   entity.Role
		wantStatus int
		wantPass   bool
	}{
		{
			name:       "自定义角色匹配通过",
			allowed:    []entity.Role{customRole, entity.RoleAdmin},
			userRole:   customRole,
			wantStatus: http.StatusOK,
			wantPass:   true,
		},
		{
			name:       "自定义角色不匹配被拒绝",
			allowed:    []entity.Role{customRole},
			userRole:   entity.RoleReadonly,
			wantStatus: http.StatusForbidden,
			wantPass:   false,
		},
		{
			name:       "空角色列表拒绝所有",
			allowed:    []entity.Role{},
			userRole:   entity.RoleAdmin,
			wantStatus: http.StatusForbidden,
			wantPass:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			_, r := gin.CreateTestContext(w)

			passed := false
			r.Use(func(c *gin.Context) {
				setAuthContext(c, "u1", "testuser", tt.userRole)
				c.Next()
			})
			r.Use(RequireRole(tt.allowed...))
			r.GET("/custom", func(c *gin.Context) {
				passed = true
				response.Success(c, nil)
			})

			req, _ := http.NewRequest(http.MethodGet, "/custom", nil)
			r.ServeHTTP(w, req)

			require.Equal(t, tt.wantStatus, w.Code)
			require.Equal(t, tt.wantPass, passed)
		})
	}
}
