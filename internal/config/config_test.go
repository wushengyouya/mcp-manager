package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoad_DefaultsAndEnvOverride 验证默认值加载和环境变量覆盖生效
func TestLoad_DefaultsAndEnvOverride(t *testing.T) {
	t.Setenv("MCP_SERVER_PORT", "9999")
	t.Setenv("MCP_JWT_SECRET", "test-secret")

	cfg, err := Load("/tmp/definitely-not-exists")
	require.NoError(t, err)
	require.Equal(t, 9999, cfg.Server.Port)
	require.Equal(t, "test-secret", cfg.JWT.Secret)
	require.Equal(t, "sqlite", cfg.Database.Driver)
	require.Equal(t, "all", cfg.App.Role)
	require.Equal(t, "runtime_first", cfg.Runtime.StatusSource)
	require.True(t, cfg.Runtime.StartupReconcile)
}

// TestLoad_Validate 验证非法配置会在加载阶段被拦截
func TestLoad_Validate(t *testing.T) {
	old := os.Getenv("MCP_SERVER_PORT")
	t.Cleanup(func() {
		_ = os.Setenv("MCP_SERVER_PORT", old)
	})
	_ = os.Setenv("MCP_SERVER_PORT", "70000")
	_, err := Load("/tmp/definitely-not-exists")
	require.Error(t, err)
}

// TestLoad_RuntimePlaceholders 验证运行态占位配置可解析且不影响默认行为。
func TestLoad_RuntimePlaceholders(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
app:
  role: "executor"
runtime:
  status_source: "persisted"
  startup_reconcile: false
`), 0o644))

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "executor", cfg.App.Role)
	require.Equal(t, "persisted", cfg.Runtime.StatusSource)
	require.False(t, cfg.Runtime.StartupReconcile)
	require.Equal(t, "sqlite", cfg.Database.Driver)
}
