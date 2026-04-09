package e2e

import (
	"net/http"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/tests/testutil"
	"github.com/stretchr/testify/require"
)

// TestE2E_StreamableHTTPServiceLifecycle 验证真实 HTTP 链路下的服务完整生命周期。
func TestE2E_StreamableHTTPServiceLifecycle(t *testing.T) {
	app := testutil.SetupTestApp(t)
	api := testutil.NewHTTPServer(t, app.Engine)
	testMCP := testutil.BuildMCPServer()
	defer testMCP.Close()

	client := api.Client()
	token := testutil.LoginAndGetToken(t, client, api.URL, "root", "admin123456")
	serviceID := testutil.CreateStreamableHTTPService(t, client, api.URL, token, "remote-echo-e2e", testMCP.URL)

	statusResp := testutil.GetJSON(t, client, api.URL+"/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
	statusData := statusResp["data"].(map[string]any)
	require.Equal(t, "DISCONNECTED", statusData["status"])
	require.Equal(t, "streamable_http", statusData["transport_type"])

	testutil.PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/services/"+serviceID+"/connect", nil, token, http.StatusOK)
	statusResp = testutil.GetJSON(t, client, api.URL+"/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
	statusData = statusResp["data"].(map[string]any)
	require.Equal(t, "CONNECTED", statusData["status"])
	require.Contains(t, statusData, "session_id_exists")
	require.Contains(t, statusData, "listen_active")

	testutil.PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/services/"+serviceID+"/sync-tools", nil, token, http.StatusOK)

	toolsResp := testutil.GetJSON(t, client, api.URL+"/api/v1/services/"+serviceID+"/tools", token, http.StatusOK)
	toolID := toolsResp["data"].([]any)[0].(map[string]any)["id"].(string)

	invokeResp := testutil.PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/tools/"+toolID+"/invoke", map[string]any{
		"arguments": map[string]any{"text": "hello"},
	}, token, http.StatusOK)
	result := invokeResp["data"].(map[string]any)["result"].(map[string]any)
	require.Equal(t, "streamable_http", result["transport_type"])
	payload := result["payload"].(map[string]any)
	require.Contains(t, payload["content"].([]any)[0].(map[string]any)["text"].(string), "echo:hello")

	testutil.PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/services/"+serviceID+"/disconnect", nil, token, http.StatusOK)
	statusResp = testutil.GetJSON(t, client, api.URL+"/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
	statusData = statusResp["data"].(map[string]any)
	require.Equal(t, "DISCONNECTED", statusData["status"])

	historyResp := testutil.GetJSON(t, client, api.URL+"/api/v1/history", token, http.StatusOK)
	require.Equal(t, float64(1), historyResp["data"].(map[string]any)["total"])
}

// TestE2E_ReadonlyUserCannotModifyService 验证只读用户可以读取服务，但不能执行修改操作。
func TestE2E_AsyncInvokeLifecycle(t *testing.T) {
	cfg := testutil.DefaultTestConfig(t)
	cfg.Execution.AsyncInvokeEnabled = true
	cfg.Execution.AsyncTaskQueueSize = 8
	cfg.Execution.AsyncTaskWorkers = 1
	cfg.Execution.DefaultTaskTimeout = 100 * time.Millisecond
	app := testutil.NewTestAppBuilder(t).WithConfig(cfg).Build()
	api := testutil.NewHTTPServer(t, app.Engine)
	testMCP := testutil.BuildMCPServer()
	defer testMCP.Close()

	client := api.Client()
	token := testutil.LoginAndGetToken(t, client, api.URL, "root", "admin123456")
	serviceID := testutil.CreateStreamableHTTPService(t, client, api.URL, token, "remote-echo-e2e-async", testMCP.URL)
	testutil.PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/services/"+serviceID+"/connect", nil, token, http.StatusOK)
	testutil.PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/services/"+serviceID+"/sync-tools", nil, token, http.StatusOK)
	toolsResp := testutil.GetJSON(t, client, api.URL+"/api/v1/services/"+serviceID+"/tools", token, http.StatusOK)
	toolID := toolsResp["data"].([]any)[0].(map[string]any)["id"].(string)

	taskID := testutil.InvokeAsyncAndGetTaskID(t, client, api.URL, token, toolID, map[string]any{
		"arguments": map[string]any{"text": "hello-async-e2e", "delay_ms": 20},
	}, http.StatusAccepted)
	task := testutil.WaitForTaskState(t, client, api.URL, token, taskID, "succeeded", time.Second)
	result := task["result"].(map[string]any)
	payload := result["payload"].(map[string]any)
	require.Contains(t, payload["content"].([]any)[0].(map[string]any)["text"].(string), "echo:hello-async-e2e")
}

func TestE2E_ReadonlyUserCannotModifyService(t *testing.T) {
	app := testutil.SetupTestApp(t)
	api := testutil.NewHTTPServer(t, app.Engine)
	client := api.Client()

	adminToken := testutil.LoginAndGetToken(t, client, api.URL, "root", "admin123456")
	createUserResp := testutil.PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/users", map[string]any{
		"username": "viewer",
		"password": "viewer123456",
		"email":    "viewer@example.com",
		"role":     "readonly",
	}, adminToken, http.StatusCreated)
	require.NotEmpty(t, createUserResp["data"].(map[string]any)["id"])

	readonlyToken := testutil.LoginAndGetToken(t, client, api.URL, "viewer", "viewer123456")
	testutil.GetJSON(t, client, api.URL+"/api/v1/services", readonlyToken, http.StatusOK)

	resp := testutil.PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/services", map[string]any{
		"name":           "readonly-should-fail",
		"transport_type": "streamable_http",
		"url":            "http://127.0.0.1:65535",
		"session_mode":   "auto",
		"compat_mode":    "off",
		"listen_enabled": true,
		"timeout":        5,
	}, readonlyToken, http.StatusForbidden)
	require.Equal(t, "权限不足", resp["message"])
}
