package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mikasa/mcp-manager/tests/testutil"
	"github.com/stretchr/testify/require"
)

// TestIntegration_StreamableHTTPServiceLifecycle 验证远程服务从创建到调用工具的完整生命周期
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
