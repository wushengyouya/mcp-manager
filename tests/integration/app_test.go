package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/database"
	"github.com/mikasa/mcp-manager/internal/handler"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/router"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/mikasa/mcp-manager/scripts"
	"github.com/stretchr/testify/require"
)

// TestIntegration_StreamableHTTPServiceLifecycle 验证远程服务从创建到调用工具的完整生命周期
func TestIntegration_StreamableHTTPServiceLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, logger.Init(config.LogConfig{Level: "error", Format: "console", Output: "stdout"}))

	db, err := database.Init(config.DatabaseConfig{
		Driver:          "sqlite",
		DSN:             ":memory:",
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Hour,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, database.Migrate(db))

	userRepo := repository.NewUserRepository(db)
	serviceRepo := repository.NewMCPServiceRepository(db)
	toolRepo := repository.NewToolRepository(db)
	historyRepo := repository.NewRequestHistoryRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	require.NoError(t, scripts.EnsureAdmin(context.Background(), userRepo, "root", "admin123456", "root@example.com"))

	jwtSvc := crypto.NewJWTService("secret", "issuer", time.Hour, 24*time.Hour, crypto.NewTokenBlacklist())
	auditSink := service.NewDBAuditSink(auditRepo)
	authSvc := service.NewAuthService(userRepo, jwtSvc, auditSink)
	userSvc := service.NewUserService(userRepo, auditSink)
	manager := mcpclient.NewManager(config.AppConfig{Name: "test", Version: "1.0.0"})
	mcpSvc := service.NewMCPService(serviceRepo, toolRepo, manager, auditSink, nil)
	toolSvc := service.NewToolService(toolRepo, serviceRepo, manager, auditSink)
	invokeSvc := service.NewToolInvokeService(config.HistoryConfig{MaxBodyBytes: 4096, Compression: "none"}, toolRepo, serviceRepo, historyRepo, manager)
	auditSvc := service.NewAuditService(auditSink, auditRepo)

	engine := router.New(jwtSvc, router.Handlers{
		Auth:    handler.NewAuthHandler(authSvc),
		User:    handler.NewUserHandler(userSvc, authSvc),
		MCP:     handler.NewMCPHandler(mcpSvc),
		Tool:    handler.NewToolHandler(toolSvc, invokeSvc),
		History: handler.NewHistoryHandler(historyRepo),
		Audit:   handler.NewAuditHandler(auditSvc),
	})

	testMCP := buildMCPServer()
	defer testMCP.Close()

	token := loginAndGetToken(t, engine)

	serviceID := createService(t, engine, token, testMCP.URL)
	postJSON(t, engine, http.MethodPost, "/api/v1/services/"+serviceID+"/connect", nil, token, http.StatusOK)
	postJSON(t, engine, http.MethodPost, "/api/v1/services/"+serviceID+"/sync-tools", nil, token, http.StatusOK)

	toolsResp := getJSON(t, engine, "/api/v1/services/"+serviceID+"/tools", token, http.StatusOK)
	toolID := toolsResp["data"].([]any)[0].(map[string]any)["id"].(string)

	invokeResp := postJSON(t, engine, http.MethodPost, "/api/v1/tools/"+toolID+"/invoke", map[string]any{
		"arguments": map[string]any{"text": "hello"},
	}, token, http.StatusOK)

	data := invokeResp["data"].(map[string]any)
	require.NotNil(t, data["result"])

	historyResp := getJSON(t, engine, "/api/v1/history", token, http.StatusOK)
	require.Equal(t, float64(1), historyResp["data"].(map[string]any)["total"])
}

// buildMCPServer 构建集成测试使用的临时 MCP 服务
func buildMCPServer() *httptest.Server {
	srv := mcpserver.NewMCPServer("test-mcp", "1.0.0", mcpserver.WithToolCapabilities(true))
	srv.AddTool(mcp.NewTool("echo", mcp.WithString("text", mcp.Required())), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, _ := req.Params.Arguments.(map[string]any)
		text, _ := args["text"].(string)
		return mcp.NewToolResultText("echo:" + text), nil
	})
	return mcpserver.NewTestStreamableHTTPServer(srv, mcpserver.WithStateful(true))
}

// loginAndGetToken 登录默认管理员并返回访问令牌
func loginAndGetToken(t *testing.T, engine *gin.Engine) string {
	resp := postJSON(t, engine, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "root",
		"password": "admin123456",
	}, "", http.StatusOK)
	data := resp["data"].(map[string]any)
	return data["access_token"].(string)
}

// createService 通过 API 创建一个远程 Streamable HTTP 服务
func createService(t *testing.T, engine *gin.Engine, token, url string) string {
	resp := postJSON(t, engine, http.MethodPost, "/api/v1/services", map[string]any{
		"name":           "remote-echo",
		"transport_type": "streamable_http",
		"url":            url,
		"session_mode":   "auto",
		"compat_mode":    "off",
		"listen_enabled": true,
		"timeout":        10,
		"custom_headers": map[string]string{},
		"description":    "test",
	}, token, http.StatusCreated)
	return resp["data"].(map[string]any)["id"].(string)
}

// getJSON 发起 GET 请求并解析统一 JSON 响应
func getJSON(t *testing.T, engine *gin.Engine, path, token string, expectCode int) map[string]any {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, expectCode, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

// postJSON 发起 JSON 请求并解析统一 JSON 响应
func postJSON(t *testing.T, engine *gin.Engine, method, path string, payload any, token string, expectCode int) map[string]any {
	var buf bytes.Buffer
	if payload != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(payload))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, expectCode, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}
