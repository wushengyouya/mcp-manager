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
