package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/stretchr/testify/require"
)

func TestSetupTestAppKeepsDefaultAllBehavior(t *testing.T) {
	app := SetupTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	app.Engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "all", app.Role)
}

func TestTestAppBuilderAllowsRuntimeFactoryOverride(t *testing.T) {
	called := false
	app := NewTestAppBuilder(t).
		WithRuntimeFactory(func(appCfg config.AppConfig) *mcpclient.Manager {
			called = true
			return mcpclient.NewManager(appCfg)
		}).
		Build()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	app.Engine.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.True(t, called)
}

func TestTestAppBuilderAllowsRoleOverride(t *testing.T) {
	app := NewTestAppBuilder(t).WithAppRole("control-plane").Build()
	require.Equal(t, "control-plane", app.Role)
}

func TestSetupDualRoleTestAppProvidesControlPlaneAndExecutor(t *testing.T) {
	app := SetupDualRoleTestApp(t)
	api := NewHTTPServer(t, app.ControlPlane.Engine)
	testMCP := BuildMCPServer()
	defer testMCP.Close()

	client := api.Client()
	token := LoginAndGetToken(t, client, api.URL, "root", "admin123456")
	serviceID := CreateStreamableHTTPService(t, client, api.URL, token, "dual-role-echo", testMCP.URL)

	statusResp := GetJSON(t, client, api.URL+"/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
	statusData := statusResp["data"].(map[string]any)
	require.Equal(t, "persisted", statusData["status_source"])
	require.Equal(t, "DISCONNECTED", statusData["status"])

	PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/services/"+serviceID+"/connect", nil, token, http.StatusOK)
	PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/services/"+serviceID+"/sync-tools", nil, token, http.StatusOK)
	toolsResp := GetJSON(t, client, api.URL+"/api/v1/services/"+serviceID+"/tools", token, http.StatusOK)
	toolID := toolsResp["data"].([]any)[0].(map[string]any)["id"].(string)

	invokeResp := PostJSON(t, client, http.MethodPost, api.URL+"/api/v1/tools/"+toolID+"/invoke", map[string]any{
		"arguments": map[string]any{"text": "hello-dual-role"},
	}, token, http.StatusOK)
	payload := invokeResp["data"].(map[string]any)["result"].(map[string]any)["payload"].(map[string]any)
	require.NotNil(t, payload["content"])

	app.ExecutorRPC.Close()
	statusResp = GetJSON(t, client, api.URL+"/api/v1/services/"+serviceID+"/status", token, http.StatusOK)
	statusData = statusResp["data"].(map[string]any)
	require.Equal(t, "snapshot", statusData["status_source"])
	require.Equal(t, "fresh", statusData["snapshot_freshness"])
}
