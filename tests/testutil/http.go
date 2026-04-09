package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// GetJSON 发起 GET 请求并解析统一 JSON 响应。
func GetJSON(t *testing.T, client *http.Client, url, token string, expectCode int) map[string]any {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doJSON(t, client, req, expectCode)
}

// PostJSON 发起 JSON 请求并解析统一 JSON 响应。
func PostJSON(t *testing.T, client *http.Client, method, url string, payload any, token string, expectCode int) map[string]any {
	t.Helper()

	var body bytes.Buffer
	if payload != nil {
		require.NoError(t, json.NewEncoder(&body).Encode(payload))
	}

	req, err := http.NewRequest(method, url, &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doJSON(t, client, req, expectCode)
}

// LoginAndGetToken 登录并返回访问令牌。
func LoginAndGetToken(t *testing.T, client *http.Client, baseURL, username, password string) string {
	t.Helper()

	resp := PostJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/login", map[string]any{
		"username": username,
		"password": password,
	}, "", http.StatusOK)
	data := resp["data"].(map[string]any)
	return data["access_token"].(string)
}

// CreateStreamableHTTPService 通过 API 创建一个远程服务。
func CreateStreamableHTTPService(t *testing.T, client *http.Client, baseURL, token, name, url string) string {
	t.Helper()

	resp := PostJSON(t, client, http.MethodPost, baseURL+"/api/v1/services", map[string]any{
		"name":           name,
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

// InvokeAsyncAndGetTaskID 提交异步调用并返回任务 ID。
func InvokeAsyncAndGetTaskID(t *testing.T, client *http.Client, baseURL, token, toolID string, payload map[string]any, expectCode int) string {
	t.Helper()
	resp := PostJSON(t, client, http.MethodPost, baseURL+"/api/v1/tools/"+toolID+"/invoke-async", payload, token, expectCode)
	return resp["data"].(map[string]any)["id"].(string)
}

// GetTask 查询异步任务状态。
func GetTask(t *testing.T, client *http.Client, baseURL, token, taskID string, expectCode int) map[string]any {
	t.Helper()
	return GetJSON(t, client, baseURL+"/api/v1/tasks/"+taskID, token, expectCode)
}

// CancelTask 取消异步任务。
func CancelTask(t *testing.T, client *http.Client, baseURL, token, taskID string, expectCode int) map[string]any {
	t.Helper()
	return PostJSON(t, client, http.MethodPost, baseURL+"/api/v1/tasks/"+taskID+"/cancel", nil, token, expectCode)
}

// WaitForTaskState 轮询任务直到进入指定状态。
func WaitForTaskState(t *testing.T, client *http.Client, baseURL, token, taskID, want string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := GetTask(t, client, baseURL, token, taskID, http.StatusOK)
		data := resp["data"].(map[string]any)
		if fmt.Sprint(data["status"]) == want {
			return data
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("任务 %s 未在 %s 内进入状态 %s", taskID, timeout, want)
	return nil
}

// doJSON 发送请求并断言统一 JSON 响应。
func doJSON(t *testing.T, client *http.Client, req *http.Request, expectCode int) map[string]any {
	t.Helper()

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.Equal(t, expectCode, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}
