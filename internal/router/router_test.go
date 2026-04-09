package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/handler"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/service"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/stretchr/testify/require"
)

type routerAuthServiceStub struct{}

func (routerAuthServiceStub) Login(context.Context, string, string, string, string) (*appcrypto.TokenPair, *entity.User, error) {
	return &appcrypto.TokenPair{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 3600}, &entity.User{
		Base:        entity.Base{ID: "user-1"},
		Username:    "tester",
		Email:       "tester@example.com",
		Role:        entity.RoleAdmin,
		IsActive:    true,
		LastLoginAt: nil,
	}, nil
}

func (routerAuthServiceStub) Logout(context.Context, string, string, string, string, string, string) error {
	return nil
}

func (routerAuthServiceStub) Refresh(context.Context, string) (*appcrypto.TokenPair, error) {
	return &appcrypto.TokenPair{AccessToken: "new-access", RefreshToken: "new-refresh", ExpiresIn: 3600}, nil
}

func (routerAuthServiceStub) ChangePassword(context.Context, string, string, string, string, string, string) error {
	return nil
}

type routerUserServiceStub struct{}

func (routerUserServiceStub) Create(context.Context, service.CreateUserInput, service.AuditEntry) (*entity.User, error) {
	return &entity.User{Base: entity.Base{ID: "user-created"}, Username: "created", Email: "created@example.com", Role: entity.RoleReadonly, IsActive: true}, nil
}

func (routerUserServiceStub) Update(context.Context, string, service.UpdateUserInput, service.AuditEntry) (*entity.User, error) {
	return &entity.User{Base: entity.Base{ID: "user-updated"}, Username: "updated", Email: "updated@example.com", Role: entity.RoleOperator, IsActive: true}, nil
}

func (routerUserServiceStub) Delete(context.Context, string, string, service.AuditEntry) error {
	return nil
}

func (routerUserServiceStub) Get(context.Context, string) (*entity.User, error) {
	return &entity.User{Base: entity.Base{ID: "user-get"}, Username: "get"}, nil
}

func (routerUserServiceStub) List(context.Context, repository.UserListFilter) ([]entity.User, int64, error) {
	return []entity.User{{Base: entity.Base{ID: "user-1"}, Username: "tester"}}, 1, nil
}

type routerMCPServiceStub struct{}

func (routerMCPServiceStub) Create(context.Context, service.CreateMCPServiceInput, service.AuditEntry) (*entity.MCPService, error) {
	return &entity.MCPService{Base: entity.Base{ID: "svc-1"}, Name: "svc-1", TransportType: entity.TransportTypeStreamableHTTP, URL: "http://svc-1.test/mcp"}, nil
}

func (routerMCPServiceStub) Update(context.Context, string, service.CreateMCPServiceInput, service.AuditEntry) (*entity.MCPService, error) {
	return &entity.MCPService{Base: entity.Base{ID: "svc-1"}, Name: "svc-1", TransportType: entity.TransportTypeStreamableHTTP, URL: "http://svc-1.test/mcp"}, nil
}

func (routerMCPServiceStub) Delete(context.Context, string, service.AuditEntry) error { return nil }

func (routerMCPServiceStub) Get(context.Context, string) (*entity.MCPService, error) {
	return &entity.MCPService{Base: entity.Base{ID: "svc-1"}, Name: "svc-1", TransportType: entity.TransportTypeStreamableHTTP, URL: "http://svc-1.test/mcp"}, nil
}

func (routerMCPServiceStub) List(context.Context, repository.MCPServiceListFilter) ([]entity.MCPService, int64, error) {
	return []entity.MCPService{{Base: entity.Base{ID: "svc-1"}, Name: "svc-1", TransportType: entity.TransportTypeStreamableHTTP, URL: "http://svc-1.test/mcp"}}, 1, nil
}

func (routerMCPServiceStub) Connect(context.Context, string, service.AuditEntry) (mcpclient.RuntimeStatus, error) {
	return mcpclient.RuntimeStatus{ServiceID: "svc-1", Status: entity.ServiceStatusConnected}, nil
}

func (routerMCPServiceStub) Disconnect(context.Context, string, service.AuditEntry) error { return nil }

func (routerMCPServiceStub) Status(context.Context, string) (map[string]any, error) {
	return map[string]any{"id": "svc-1", "status": entity.ServiceStatusConnected}, nil
}

type routerToolServiceStub struct{}

func (routerToolServiceStub) Sync(context.Context, string, service.AuditEntry) ([]entity.Tool, error) {
	return []entity.Tool{{Base: entity.Base{ID: "tool-1"}, Name: "search"}}, nil
}

func (routerToolServiceStub) ListByService(context.Context, string) ([]entity.Tool, error) {
	return []entity.Tool{{Base: entity.Base{ID: "tool-1"}, Name: "search"}}, nil
}

func (routerToolServiceStub) Get(context.Context, string) (*entity.Tool, error) {
	return &entity.Tool{Base: entity.Base{ID: "tool-1"}, Name: "search"}, nil
}

type routerToolInvokeServiceStub struct{}

func (routerToolInvokeServiceStub) Invoke(context.Context, string, map[string]any, service.AuditEntry) (*service.ToolInvokeResult, error) {
	return &service.ToolInvokeResult{Result: map[string]any{"ok": true}, DurationMS: 1}, nil
}

func (routerToolInvokeServiceStub) InvokeAsync(context.Context, string, map[string]any, time.Duration, service.AuditEntry) (*service.AsyncInvokeTask, error) {
	return &service.AsyncInvokeTask{ID: "task-1", Status: service.AsyncTaskStatusPending}, nil
}

func (routerToolInvokeServiceStub) GetTask(context.Context, string, service.AuditEntry) (*service.AsyncInvokeTask, error) {
	return &service.AsyncInvokeTask{ID: "task-1", Status: service.AsyncTaskStatusRunning}, nil
}

func (routerToolInvokeServiceStub) CancelTask(context.Context, string, service.AuditEntry) (*service.AsyncInvokeTask, error) {
	return &service.AsyncInvokeTask{ID: "task-1", Status: service.AsyncTaskStatusCancelled}, nil
}

func (routerToolInvokeServiceStub) TaskStats(context.Context, service.AuditEntry) (*service.AsyncTaskStats, error) {
	return &service.AsyncTaskStats{Pending: 1}, nil
}

func (routerToolInvokeServiceStub) Stop(context.Context) error { return nil }

type routerHistoryRepoStub struct{}

func (routerHistoryRepoStub) Create(context.Context, *entity.RequestHistory) error { return nil }

func (routerHistoryRepoStub) GetByID(context.Context, string) (*entity.RequestHistory, error) {
	return &entity.RequestHistory{ID: "history-1", UserID: "user-1", ToolName: "search", Status: entity.RequestStatusSuccess}, nil
}

func (routerHistoryRepoStub) List(context.Context, repository.HistoryListFilter) ([]entity.RequestHistory, int64, error) {
	return []entity.RequestHistory{{ID: "history-1", UserID: "user-1", ToolName: "search", Status: entity.RequestStatusSuccess}}, 1, nil
}

type routerAuditServiceStub struct{}

func (routerAuditServiceStub) Record(context.Context, service.AuditEntry) error { return nil }

func (routerAuditServiceStub) List(context.Context, repository.AuditListFilter) ([]map[string]any, int64, error) {
	return []map[string]any{{"id": "audit-1", "action": "login"}}, 1, nil
}

func (routerAuditServiceStub) ExportCSV(context.Context, repository.AuditListFilter) ([]byte, error) {
	return []byte("id,action\naudit-1,login\n"), nil
}

type routerEnv struct {
	engine        *gin.Engine
	jwtSvc        *appcrypto.JWTService
	adminToken    string
	operatorToken string
	readonlyToken string
}

func setupRouterEnv(t *testing.T) *routerEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	jwtSvc := appcrypto.NewJWTService("router-test-secret", "router-test", time.Hour, 24*time.Hour, nil)
	adminPair, err := jwtSvc.GenerateTokenPair("admin-1", "admin", string(entity.RoleAdmin))
	require.NoError(t, err)
	operatorPair, err := jwtSvc.GenerateTokenPair("operator-1", "operator", string(entity.RoleOperator))
	require.NoError(t, err)
	readonlyPair, err := jwtSvc.GenerateTokenPair("readonly-1", "readonly", string(entity.RoleReadonly))
	require.NoError(t, err)

	engine := New(jwtSvc, Handlers{
		Auth:    handler.NewAuthHandler(routerAuthServiceStub{}),
		User:    handler.NewUserHandler(routerUserServiceStub{}, routerAuthServiceStub{}),
		MCP:     handler.NewMCPHandler(routerMCPServiceStub{}),
		Tool:    handler.NewToolHandler(routerToolServiceStub{}, routerToolInvokeServiceStub{}),
		History: handler.NewHistoryHandler(routerHistoryRepoStub{}),
		Audit:   handler.NewAuditHandler(routerAuditServiceStub{}),
	})
	return &routerEnv{
		engine:        engine,
		jwtSvc:        jwtSvc,
		adminToken:    adminPair.AccessToken,
		operatorToken: operatorPair.AccessToken,
		readonlyToken: readonlyPair.AccessToken,
	}
}

func routerRequest(method, path, body, token string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func TestNew_PublicRoutesAndCORS(t *testing.T) {
	env := setupRouterEnv(t)

	req := routerRequest(http.MethodGet, "/health", "", "")
	w := httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = routerRequest(http.MethodPost, "/api/v1/auth/login", `{"username":"u","password":"p"}`, "")
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = routerRequest(http.MethodPost, "/api/v1/auth/refresh", `{"refresh_token":"rt"}`, "")
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = routerRequest(http.MethodOptions, "/api/v1/services", "", "")
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)
	require.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestNew_AuthenticatedReadRoutesRequireTokenButAllowReadonly(t *testing.T) {
	env := setupRouterEnv(t)

	req := routerRequest(http.MethodGet, "/api/v1/services", "", "")
	w := httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)

	req = routerRequest(http.MethodGet, "/api/v1/services", "", env.readonlyToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = routerRequest(http.MethodGet, "/api/v1/services/svc-1/status", "", env.readonlyToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestNew_ModifyRoutesRequireModifyRole(t *testing.T) {
	env := setupRouterEnv(t)

	body := `{"name":"svc-1","transport_type":"streamable_http","url":"http://svc-1.test/mcp"}`
	req := routerRequest(http.MethodPost, "/api/v1/services", body, env.operatorToken)
	w := httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	req = routerRequest(http.MethodPost, "/api/v1/services", body, env.readonlyToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	req = routerRequest(http.MethodPost, "/api/v1/tools/tool-1/invoke", `{"arguments":{"q":"hello"}}`, env.operatorToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestNew_AsyncTaskRoutesRespectPermissions(t *testing.T) {
	env := setupRouterEnv(t)

	req := routerRequest(http.MethodPost, "/api/v1/tools/tool-1/invoke-async", `{"arguments":{"q":"hello"}}`, env.operatorToken)
	w := httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	req = routerRequest(http.MethodPost, "/api/v1/tools/tool-1/invoke-async", `{"arguments":{"q":"hello"}}`, env.readonlyToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	req = routerRequest(http.MethodGet, "/api/v1/tasks/task-1", "", env.readonlyToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = routerRequest(http.MethodPost, "/api/v1/tasks/task-1/cancel", ``, env.operatorToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
}

func TestNew_AdminRoutesRequireAdmin(t *testing.T) {
	env := setupRouterEnv(t)

	req := routerRequest(http.MethodGet, "/api/v1/users", "", env.adminToken)
	w := httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = routerRequest(http.MethodGet, "/api/v1/users", "", env.operatorToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	req = routerRequest(http.MethodGet, "/api/v1/tasks/stats", "", env.adminToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = routerRequest(http.MethodGet, "/api/v1/tasks/stats", "", env.operatorToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	req = routerRequest(http.MethodGet, "/api/v1/audit-logs/export?action=login", "", env.adminToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "attachment; filename=audit_logs.csv", w.Header().Get("Content-Disposition"))
}

func TestNew_LogoutAndChangePasswordRoutes(t *testing.T) {
	env := setupRouterEnv(t)

	req := routerRequest(http.MethodPost, "/api/v1/auth/logout", `{}`, env.adminToken)
	w := httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = routerRequest(http.MethodPut, "/api/v1/users/admin-1/password", `{"old_password":"old","new_password":"newpass123"}`, env.adminToken)
	w = httptest.NewRecorder()
	env.engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, float64(0), body["code"])
}
