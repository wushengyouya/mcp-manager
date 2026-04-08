package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	require.Equal(t, "postgres", cfg.Database.Driver)
	require.Equal(t, "postgres://postgres:postgres@127.0.0.1:5432/mcp_manager?sslmode=disable", cfg.Database.DSN)
	require.Equal(t, "all", cfg.App.Role)
	require.Equal(t, "runtime_first", cfg.Runtime.StatusSource)
	require.True(t, cfg.Runtime.StartupReconcile)
	require.True(t, cfg.Redis.Enabled)
	require.False(t, cfg.Runtime.SnapshotEnabled)
	require.Equal(t, 30*time.Second, cfg.Runtime.SnapshotTTL)
	require.Equal(t, time.Duration(0), cfg.Runtime.IdleTimeout)
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
  snapshot_enabled: true
  snapshot_ttl: 45s
  idle_timeout: 2m
redis:
  enabled: true
  addr: "127.0.0.1:6380"
`), 0o644))

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "executor", cfg.App.Role)
	require.Equal(t, "persisted", cfg.Runtime.StatusSource)
	require.False(t, cfg.Runtime.StartupReconcile)
	require.True(t, cfg.Runtime.SnapshotEnabled)
	require.Equal(t, 45*time.Second, cfg.Runtime.SnapshotTTL)
	require.Equal(t, 2*time.Minute, cfg.Runtime.IdleTimeout)
	require.True(t, cfg.Redis.Enabled)
	require.Equal(t, "127.0.0.1:6380", cfg.Redis.Addr)
	require.Equal(t, "postgres", cfg.Database.Driver)
}

func TestValidate_AllowsPostgresDriver(t *testing.T) {
	cfg, err := Load("/tmp/definitely-not-exists")
	require.NoError(t, err)

	cfg.Database.Driver = "postgres"
	cfg.Database.DSN = "postgres://tester:secret@127.0.0.1:5432/mcp_manager?sslmode=disable"
	require.NoError(t, cfg.Validate())
}

func TestLoad_AllowsExplicitSQLiteFallback(t *testing.T) {
	t.Setenv("MCP_DATABASE_DRIVER", "sqlite")
	t.Setenv("MCP_DATABASE_DSN", "data/mcp_manager.db")

	cfg, err := Load("/tmp/definitely-not-exists")
	require.NoError(t, err)
	require.Equal(t, "sqlite", cfg.Database.Driver)
	require.Equal(t, "data/mcp_manager.db", cfg.Database.DSN)
}
