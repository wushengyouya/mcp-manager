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
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/service"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/response"
	"github.com/stretchr/testify/require"
)

type mcpServiceStub struct {
	connectStatus mcpclient.RuntimeStatus
	connectErr    error
	connectActor  service.AuditEntry
	disconnectErr error
}

func (s *mcpServiceStub) Create(context.Context, service.CreateMCPServiceInput, service.AuditEntry) (*entity.MCPService, error) {
	return &entity.MCPService{}, nil
}

func (s *mcpServiceStub) Update(context.Context, string, service.CreateMCPServiceInput, service.AuditEntry) (*entity.MCPService, error) {
	return &entity.MCPService{}, nil
}

func (s *mcpServiceStub) Delete(context.Context, string, service.AuditEntry) error { return nil }

func (s *mcpServiceStub) Get(context.Context, string) (*entity.MCPService, error) {
	return &entity.MCPService{}, nil
}

func (s *mcpServiceStub) List(context.Context, repository.MCPServiceListFilter) ([]entity.MCPService, int64, error) {
	return nil, 0, nil
}

func (s *mcpServiceStub) Connect(_ context.Context, _ string, actor service.AuditEntry) (mcpclient.RuntimeStatus, error) {
	s.connectActor = actor
	return s.connectStatus, s.connectErr
}

func (s *mcpServiceStub) Disconnect(context.Context, string, service.AuditEntry) error {
	return s.disconnectErr
}

func (s *mcpServiceStub) Status(context.Context, string) (map[string]any, error) {
	return map[string]any{"status": entity.ServiceStatusConnected}, nil
}

type toolServiceStub struct {
	syncActor service.AuditEntry
	syncErr   error
}

func (s *toolServiceStub) Sync(_ context.Context, _ string, actor service.AuditEntry) ([]entity.Tool, error) {
	s.syncActor = actor
	if s.syncErr != nil {
		return nil, s.syncErr
	}
	return []entity.Tool{{Base: entity.Base{ID: "tool-1"}, Name: "search"}}, nil
}

func (s *toolServiceStub) ListByService(context.Context, string) ([]entity.Tool, error) {
	return []entity.Tool{{Base: entity.Base{ID: "tool-1"}, Name: "search"}}, nil
}

func (s *toolServiceStub) Get(context.Context, string) (*entity.Tool, error) {
	return &entity.Tool{Base: entity.Base{ID: "tool-1"}, Name: "search"}, nil
}

type toolInvokeServiceStub struct {
	toolID string
	args   map[string]any
	actor  service.AuditEntry
	err    error
}

func (s *toolInvokeServiceStub) Invoke(_ context.Context, toolID string, args map[string]any, actor service.AuditEntry) (*service.ToolInvokeResult, error) {
	s.toolID = toolID
	s.args = args
	s.actor = actor
	if s.err != nil {
		return nil, s.err
	}
	return &service.ToolInvokeResult{Result: map[string]any{"ok": true}, DurationMS: 1}, nil
}

func (s *toolInvokeServiceStub) InvokeAsync(_ context.Context, toolID string, args map[string]any, _ time.Duration, actor service.AuditEntry) (*service.AsyncInvokeTask, error) {
	s.toolID = toolID
	s.args = args
	s.actor = actor
	if s.err != nil {
		return nil, s.err
	}
	return &service.AsyncInvokeTask{ID: "task-1", Status: service.AsyncTaskStatusPending}, nil
}

func (s *toolInvokeServiceStub) GetTask(context.Context, string, service.AuditEntry) (*service.AsyncInvokeTask, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &service.AsyncInvokeTask{ID: "task-1", Status: service.AsyncTaskStatusRunning}, nil
}

func (s *toolInvokeServiceStub) CancelTask(context.Context, string, service.AuditEntry) (*service.AsyncInvokeTask, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &service.AsyncInvokeTask{ID: "task-1", Status: service.AsyncTaskStatusCancelled}, nil
}

func (s *toolInvokeServiceStub) TaskStats(context.Context, service.AuditEntry) (*service.AsyncTaskStats, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &service.AsyncTaskStats{Pending: 1}, nil
}

func (s *toolInvokeServiceStub) Stop(context.Context) error { return nil }

func stubActorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("current_user_id", "u-1")
		c.Set("current_username", "root")
		c.Set("current_role", string(entity.RoleAdmin))
		c.Next()
	}
}

func TestMCPHandlerConnect_SuccessAndError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &mcpServiceStub{
		connectStatus: mcpclient.RuntimeStatus{ServiceID: "svc-1", Status: entity.ServiceStatusConnected},
	}
	h := NewMCPHandler(stub)
	r := gin.New()
	r.Use(stubActorMiddleware())
	r.POST("/services/:id/connect", h.Connect)

	req := httptest.NewRequest(http.MethodPost, "/services/svc-1/connect", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "u-1", stub.connectActor.UserID)
	require.Equal(t, "root", stub.connectActor.Username)

	stub.connectErr = response.NewBizError(http.StatusBadGateway, response.CodeServiceConnectFailed, "连接失败", nil)
	req = httptest.NewRequest(http.MethodPost, "/services/svc-1/connect", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadGateway, w.Code)
}

func TestToolHandlerSync_UsesActorAndPropagatesError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	toolStub := &toolServiceStub{}
	invokeStub := &toolInvokeServiceStub{}
	h := NewToolHandler(toolStub, invokeStub)
	r := gin.New()
	r.Use(stubActorMiddleware())
	r.POST("/services/:id/sync-tools", h.Sync)

	req := httptest.NewRequest(http.MethodPost, "/services/svc-1/sync-tools", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "u-1", toolStub.syncActor.UserID)

	toolStub.syncErr = response.NewBizError(http.StatusConflict, response.CodeConflict, "服务错误", nil)
	req = httptest.NewRequest(http.MethodPost, "/services/svc-1/sync-tools", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusConflict, w.Code)
}

func TestToolHandlerInvoke_BadJSONAndSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	toolStub := &toolServiceStub{}
	invokeStub := &toolInvokeServiceStub{}
	h := NewToolHandler(toolStub, invokeStub)
	r := gin.New()
	r.Use(stubActorMiddleware())
	r.POST("/tools/:id/invoke", h.Invoke)

	req := httptest.NewRequest(http.MethodPost, "/tools/tool-1/invoke", strings.NewReader(`{"arguments":`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/tools/tool-1/invoke", strings.NewReader(`{"arguments":{"q":"hello"}}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "tool-1", invokeStub.toolID)
	require.Equal(t, map[string]any{"q": "hello"}, invokeStub.args)
	require.Equal(t, "u-1", invokeStub.actor.UserID)
}

func TestAuthHandlerRefreshAndChangePasswordErrorPaths(t *testing.T) {
	env := setupTestEnv(t)
	_ = loginAsAdmin(t, env.router)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refresh_token":`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)

	hashed, err := appcrypto.HashPassword("otherpass123")
	require.NoError(t, err)
	user := &entity.User{
		Username:     "other",
		Password:     hashed,
		Email:        "other@example.com",
		Role:         entity.RoleReadonly,
		IsActive:     true,
		IsFirstLogin: true,
	}
	require.NoError(t, env.userRepo.Create(context.Background(), user))

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"other","password":"otherpass123"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	env.router.ServeHTTP(loginResp, loginReq)
	require.Equal(t, http.StatusOK, loginResp.Code)
	var loginBody map[string]any
	require.NoError(t, json.Unmarshal(loginResp.Body.Bytes(), &loginBody))
	readonlyToken := loginBody["data"].(map[string]any)["access_token"].(string)

	body := `{"old_password":"otherpass123","new_password":"newpass123456"}`
	req = authRequest(http.MethodPut, "/api/v1/users/root/password", body, readonlyToken)
	w = httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestToolHandlerInvoke_ResponseShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewToolHandler(&toolServiceStub{}, &toolInvokeServiceStub{})
	r := gin.New()
	r.Use(stubActorMiddleware())
	r.POST("/tools/:id/invoke", h.Invoke)

	req := httptest.NewRequest(http.MethodPost, "/tools/tool-1/invoke", strings.NewReader(`{"arguments":{"q":"hello"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, float64(0), resp["code"])
}

func TestToolHandlerInvokeAsyncAndCancel_ResponseShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	invokeStub := &toolInvokeServiceStub{}
	h := NewToolHandler(&toolServiceStub{}, invokeStub)
	r := gin.New()
	r.Use(stubActorMiddleware())
	r.POST("/tools/:id/invoke-async", h.InvokeAsync)
	r.POST("/tasks/:id/cancel", h.CancelTask)
	r.GET("/tasks/:id", h.GetTask)

	req := httptest.NewRequest(http.MethodPost, "/tools/tool-1/invoke-async", strings.NewReader(`{"arguments":{"q":"hello"},"timeout_ms":50}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, "tool-1", invokeStub.toolID)
	require.Equal(t, entity.RoleAdmin, invokeStub.actor.Role)

	req = httptest.NewRequest(http.MethodGet, "/tasks/task-1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	req = httptest.NewRequest(http.MethodPost, "/tasks/task-1/cancel", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
}
