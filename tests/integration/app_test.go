package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/bootstrap"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/rpc"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/response"
	"github.com/mikasa/mcp-manager/tests/pgtest"
	"github.com/mikasa/mcp-manager/tests/testutil"
	"github.com/stretchr/testify/require"
)

type dualRoleHarness struct {
	Engine         *gin.Engine
	ExecutorServer *httptest.Server
	cleanup        func()
}

type rpcExecutorHarness struct {
	adapter *service.LocalRuntimeAdapter
}

func (h rpcExecutorHarness) Connect(ctx context.Context, serviceItem *entity.MCPService) (mcpclient.RuntimeStatus, error) {
	return h.adapter.Connect(ctx, serviceItem)
}

func (h rpcExecutorHarness) Disconnect(ctx context.Context, serviceID string) error {
	return h.adapter.Disconnect(ctx, serviceID)
}

func (h rpcExecutorHarness) GetStatus(_ context.Context, serviceID string) (mcpclient.RuntimeStatus, bool, error) {
	status, ok := h.adapter.GetStatus(serviceID)
	return status, ok, nil
}

func (h rpcExecutorHarness) ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	return h.adapter.ListTools(ctx, serviceID)
}

func (h rpcExecutorHarness) CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	return h.adapter.CallTool(ctx, serviceID, name, args)
}

func (h rpcExecutorHarness) Ping(context.Context) error {
	return nil
}

// TestIntegration_StreamableHTTPServiceLifecycle 验证 all 模式下远程服务从创建到调用工具的完整生命周期。
func TestIntegration_StreamableHTTPServiceLifecycle(t *testing.T) {
	app := testutil.SetupTestApp(t)
	testMCP := testutil.BuildMCPServer()
	defer testMCP.Close()

	token := loginAndGetToken(t, app.Engine)

	serviceID := createService(t, app.Engine, token, testMCP.URL)
	statusResp := getJSON(t, app.Engine, "/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
	statusData := statusResp["data"].(map[string]any)
	require.Equal(t, "DISCONNECTED", statusData["status"])
	require.Equal(t, "streamable_http", statusData["transport_type"])

	postJSON(t, app.Engine, http.MethodPost, "/api/v1/services/"+serviceID+"/connect", nil, token, http.StatusOK)
	statusResp = getJSON(t, app.Engine, "/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
	statusData = statusResp["data"].(map[string]any)
	require.Equal(t, "CONNECTED", statusData["status"])
	require.Equal(t, "streamable_http", statusData["transport_type"])
	require.Contains(t, statusData, "session_id_exists")
	require.Contains(t, statusData, "listen_active")

	postJSON(t, app.Engine, http.MethodPost, "/api/v1/services/"+serviceID+"/sync-tools", nil, token, http.StatusOK)

	toolsResp := getJSON(t, app.Engine, "/api/v1/services/"+serviceID+"/tools", token, http.StatusOK)
	toolID := toolsResp["data"].([]any)[0].(map[string]any)["id"].(string)

	invokeResp := postJSON(t, app.Engine, http.MethodPost, "/api/v1/tools/"+toolID+"/invoke", map[string]any{
		"arguments": map[string]any{"text": "hello"},
	}, token, http.StatusOK)

	data := invokeResp["data"].(map[string]any)
	require.NotNil(t, data["result"])

	postJSON(t, app.Engine, http.MethodPost, "/api/v1/services/"+serviceID+"/disconnect", nil, token, http.StatusOK)
	statusResp = getJSON(t, app.Engine, "/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
	statusData = statusResp["data"].(map[string]any)
	require.Equal(t, "DISCONNECTED", statusData["status"])

	historyResp := getJSON(t, app.Engine, "/api/v1/history", token, http.StatusOK)
	require.Equal(t, float64(1), historyResp["data"].(map[string]any)["total"])
}

func TestIntegration_DualRoleStreamableHTTPServiceLifecycle(t *testing.T) {
	runAppMatrix(t, func(t *testing.T, dbCfg config.DatabaseConfig) {
		harness := setupDualRoleHarness(t, dbCfg)
		testMCP := testutil.BuildMCPServer()
		defer testMCP.Close()

		token := loginAndGetToken(t, harness.Engine)
		serviceID := createService(t, harness.Engine, token, testMCP.URL)

		statusResp := getJSON(t, harness.Engine, "/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
		statusData := statusResp["data"].(map[string]any)
		require.Equal(t, "persisted", statusData["status_source"])
		require.Equal(t, "DISCONNECTED", statusData["status"])

		postJSON(t, harness.Engine, http.MethodPost, "/api/v1/services/"+serviceID+"/connect", nil, token, http.StatusOK)

		statusResp = getJSON(t, harness.Engine, "/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
		statusData = statusResp["data"].(map[string]any)
		require.Equal(t, "runtime", statusData["status_source"])
		require.Equal(t, "CONNECTED", statusData["status"])
		require.Equal(t, "CONNECTED", statusData["persisted_status"])
		require.Contains(t, statusData, "session_id_exists")

		postJSON(t, harness.Engine, http.MethodPost, "/api/v1/services/"+serviceID+"/sync-tools", nil, token, http.StatusOK)
		toolsResp := getJSON(t, harness.Engine, "/api/v1/services/"+serviceID+"/tools", token, http.StatusOK)
		toolID := toolsResp["data"].([]any)[0].(map[string]any)["id"].(string)

		invokeResp := postJSON(t, harness.Engine, http.MethodPost, "/api/v1/tools/"+toolID+"/invoke", map[string]any{
			"arguments": map[string]any{"text": "hello-dual-role"},
		}, token, http.StatusOK)
		payload := invokeResp["data"].(map[string]any)["result"].(map[string]any)["payload"].(map[string]any)
		require.NotNil(t, payload["content"])

		postJSON(t, harness.Engine, http.MethodPost, "/api/v1/services/"+serviceID+"/disconnect", nil, token, http.StatusOK)
		statusResp = getJSON(t, harness.Engine, "/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
		statusData = statusResp["data"].(map[string]any)
		require.Equal(t, "DISCONNECTED", statusData["status"])
		require.Equal(t, "persisted", statusData["status_source"])
	})
}

func TestIntegration_DualRoleStatusFallsBackToSnapshotWhenExecutorUnavailable(t *testing.T) {
	runAppMatrix(t, func(t *testing.T, dbCfg config.DatabaseConfig) {
		harness := setupDualRoleHarness(t, dbCfg)
		testMCP := testutil.BuildMCPServer()
		defer testMCP.Close()

		token := loginAndGetToken(t, harness.Engine)
		serviceID := createService(t, harness.Engine, token, testMCP.URL)
		postJSON(t, harness.Engine, http.MethodPost, "/api/v1/services/"+serviceID+"/connect", nil, token, http.StatusOK)
		postJSON(t, harness.Engine, http.MethodPost, "/api/v1/services/"+serviceID+"/sync-tools", nil, token, http.StatusOK)
		toolsResp := getJSON(t, harness.Engine, "/api/v1/services/"+serviceID+"/tools", token, http.StatusOK)
		toolID := toolsResp["data"].([]any)[0].(map[string]any)["id"].(string)

		harness.ExecutorServer.Close()

		statusResp := getJSON(t, harness.Engine, "/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
		statusData := statusResp["data"].(map[string]any)
		require.Equal(t, "snapshot", statusData["status_source"])
		require.Equal(t, "fresh", statusData["snapshot_freshness"])
		require.Equal(t, "CONNECTED", statusData["status"])

		syncResp := postJSON(t, harness.Engine, http.MethodPost, "/api/v1/services/"+serviceID+"/sync-tools", nil, token, http.StatusBadGateway)
		require.Equal(t, float64(response.CodeToolInvokeFailed), syncResp["code"])

		invokeResp := postJSON(t, harness.Engine, http.MethodPost, "/api/v1/tools/"+toolID+"/invoke", map[string]any{
			"arguments": map[string]any{"text": "executor-down"},
		}, token, http.StatusBadGateway)
		require.Equal(t, float64(response.CodeToolInvokeFailed), invokeResp["code"])
	})
}

func TestIntegration_AppSmokeMatrix(t *testing.T) {
	runAppMatrix(t, func(t *testing.T, dbCfg config.DatabaseConfig) {
		app := testutil.NewTestAppBuilder(t).WithDatabaseConfig(dbCfg).Build()
		token := loginAndGetToken(t, app.Engine)
		serviceID := createService(t, app.Engine, token, "http://smoke.test/mcp")

		getJSON(t, app.Engine, "/api/v1/services/"+serviceID, token, http.StatusOK)
		listResp := getJSON(t, app.Engine, "/api/v1/services", token, http.StatusOK)
		require.Equal(t, float64(1), listResp["data"].(map[string]any)["total"])
	})
}

func setupDualRoleHarness(t *testing.T, dbCfg config.DatabaseConfig) *dualRoleHarness {
	t.Helper()

	mini := miniredis.RunT(t)
	cfg := testutil.DefaultTestConfig(t)
	cfg.Database = cloneDatabaseConfig(t, dbCfg)
	cfg.App.Role = "control-plane"
	cfg.Redis.Enabled = true
	cfg.Redis.Addr = mini.Addr()
	cfg.Runtime.SnapshotEnabled = true
	cfg.Runtime.SnapshotTTL = time.Minute
	cfg.RPC.Enabled = true
	cfg.RPC.ExecutorTarget = "http://executor.test"
	cfg.RPC.AuthToken = "integration-rpc-token"
	cfg.RPC.DialTimeout = time.Second
	cfg.RPC.RequestTimeout = 2 * time.Second

	executorStore := service.NewMemoryRuntimeStore()
	executorManager := mcpclient.NewManager(config.AppConfig{
		Name:    cfg.App.Name,
		Version: cfg.App.Version,
		Role:    "executor",
	})
	executorAdapter := service.NewLocalRuntimeAdapter(executorManager, executorStore)
	executorServer := httptest.NewServer(rpc.NewHandler(rpcExecutorHarness{adapter: executorAdapter}))

	app, err := bootstrap.NewBuilder(cfg).
		WithRuntimePortsFactory(func(cfg config.Config, manager *mcpclient.Manager, runtimeStore service.RuntimeStore) bootstrap.RuntimePorts {
			client := rpc.NewClient(executorServer.URL, rpc.WithHTTPClient(executorServer.Client()))
			adapter := service.NewRemoteRuntimeAdapter(
				client,
				service.WithRemoteRuntimeSnapshotStore(runtimeStore),
				service.WithRemoteRuntimeStatusTimeout(time.Second),
			)
			return bootstrap.RuntimePorts{
				Connector:    adapter,
				StatusReader: adapter,
				ToolCatalog:  adapter,
				ToolInvoker:  adapter,
			}
		}).
		Build()
	require.NoError(t, err)

	harness := &dualRoleHarness{
		Engine:         app.Engine,
		ExecutorServer: executorServer,
		cleanup: func() {
			executorServer.Close()
			app.Cleanup()
		},
	}
	t.Cleanup(harness.cleanup)
	return harness
}

func cloneDatabaseConfig(t *testing.T, dbCfg config.DatabaseConfig) config.DatabaseConfig {
	t.Helper()
	cloned := dbCfg
	if cloned.Driver == "sqlite" {
		cloned.DSN = filepath.Join(t.TempDir(), "dual-role.db")
	}
	return cloned
}

func runAppMatrix(t *testing.T, fn func(t *testing.T, dbCfg config.DatabaseConfig)) {
	t.Helper()

	t.Run("sqlite", func(t *testing.T) {
		fn(t, testutil.DefaultTestConfig(t).Database)
	})

	t.Run("postgres", func(t *testing.T) {
		fn(t, pgtest.NewPostgresDatabaseConfig(t))
	})
}

// loginAndGetToken 登录默认管理员并返回访问令牌。
func loginAndGetToken(t *testing.T, engine *gin.Engine) string {
	resp := postJSON(t, engine, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"username": "root",
		"password": "admin123456",
	}, "", http.StatusOK)
	data := resp["data"].(map[string]any)
	return data["access_token"].(string)
}

// createService 通过 API 创建一个远程 Streamable HTTP 服务。
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

// getJSON 发起 GET 请求并解析统一 JSON 响应。
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

// postJSON 发起 JSON 请求并解析统一 JSON 响应。
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
