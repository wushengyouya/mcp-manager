package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLoad_DefaultsAndEnvOverride 验证默认值加载和环境变量覆盖生效
func setRequiredSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("MCP_JWT_SECRET", "test-secret")
}

func TestLoad_DefaultsAndEnvOverride(t *testing.T) {
	setRequiredSecrets(t)
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
	require.False(t, cfg.RPC.Enabled)
	require.Equal(t, "127.0.0.1:18081", cfg.RPC.ListenAddr)
	require.Equal(t, "http://127.0.0.1:18081", cfg.RPC.ExecutorTarget)
	require.Equal(t, "dev-only-rpc-token", cfg.RPC.AuthToken)
	require.Equal(t, 3*time.Second, cfg.RPC.DialTimeout)
	require.Equal(t, 10*time.Second, cfg.RPC.RequestTimeout)
	require.Zero(t, cfg.Execution.ExecutorConcurrency)
	require.Zero(t, cfg.Execution.ServiceRateLimit)
	require.Zero(t, cfg.Execution.UserRateLimit)
	require.Equal(t, time.Minute, cfg.Execution.RateLimitWindow)
	require.False(t, cfg.Execution.AsyncInvokeEnabled)
	require.Equal(t, 64, cfg.Execution.AsyncTaskQueueSize)
	require.Equal(t, 2, cfg.Execution.AsyncTaskWorkers)
	require.Equal(t, 30*time.Second, cfg.Execution.DefaultTaskTimeout)
	require.False(t, cfg.Runtime.SnapshotEnabled)
	require.Equal(t, 30*time.Second, cfg.Runtime.SnapshotTTL)
	require.Equal(t, time.Duration(0), cfg.Runtime.IdleTimeout)
	require.False(t, cfg.Audit.AsyncEnabled)
	require.Equal(t, 256, cfg.Audit.QueueSize)
	require.False(t, cfg.Alert.AsyncEnabled)
	require.Equal(t, 64, cfg.Alert.QueueSize)
	require.False(t, cfg.History.AsyncEnabled)
	require.Equal(t, 256, cfg.History.QueueSize)
}

func TestLoad_EnvironmentOverridesRoleAndRPCFields(t *testing.T) {
	setRequiredSecrets(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
app:
  role: "all"
rpc:
  enabled: false
  listen_addr: "127.0.0.1:18081"
  executor_target: "http://127.0.0.1:18081"
  auth_token: "file-token"
`), 0o644))

	t.Setenv("MCP_APP_ROLE", "executor")
	t.Setenv("MCP_RPC_ENABLED", "true")
	t.Setenv("MCP_RPC_LISTEN_ADDR", "127.0.0.1:19081")
	t.Setenv("MCP_RPC_AUTH_TOKEN", "env-token")

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "executor", cfg.App.Role)
	require.True(t, cfg.RPC.Enabled)
	require.Equal(t, "127.0.0.1:19081", cfg.RPC.ListenAddr)
	require.Equal(t, "env-token", cfg.RPC.AuthToken)
}

// TestLoad_Validate 验证非法配置会在加载阶段被拦截
func TestLoad_Validate(t *testing.T) {
	setRequiredSecrets(t)
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
	setRequiredSecrets(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
app:
  role: "executor"
rpc:
  enabled: true
  listen_addr: "127.0.0.1:19081"
  auth_token: "executor-token"
  dial_timeout: 4s
  request_timeout: 11s
execution:
  executor_concurrency: 3
  service_rate_limit: 20
  user_rate_limit: 10
  rate_limit_window: 45s
  async_invoke_enabled: true
  async_task_queue_size: 32
  async_task_workers: 4
  default_task_timeout: 25s
runtime:
  status_source: "persisted"
  startup_reconcile: false
  snapshot_enabled: true
  snapshot_ttl: 45s
  idle_timeout: 2m
audit:
  async_enabled: true
  queue_size: 128
alert:
  async_enabled: true
  queue_size: 16
history:
  async_enabled: true
  queue_size: 96
redis:
  enabled: true
  addr: "127.0.0.1:6380"
`), 0o644))

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "executor", cfg.App.Role)
	require.True(t, cfg.RPC.Enabled)
	require.Equal(t, "127.0.0.1:19081", cfg.RPC.ListenAddr)
	require.Equal(t, "executor-token", cfg.RPC.AuthToken)
	require.Equal(t, 4*time.Second, cfg.RPC.DialTimeout)
	require.Equal(t, 11*time.Second, cfg.RPC.RequestTimeout)
	require.Equal(t, 3, cfg.Execution.ExecutorConcurrency)
	require.Equal(t, 20, cfg.Execution.ServiceRateLimit)
	require.Equal(t, 10, cfg.Execution.UserRateLimit)
	require.Equal(t, 45*time.Second, cfg.Execution.RateLimitWindow)
	require.True(t, cfg.Execution.AsyncInvokeEnabled)
	require.Equal(t, 32, cfg.Execution.AsyncTaskQueueSize)
	require.Equal(t, 4, cfg.Execution.AsyncTaskWorkers)
	require.Equal(t, 25*time.Second, cfg.Execution.DefaultTaskTimeout)
	require.Equal(t, "persisted", cfg.Runtime.StatusSource)
	require.False(t, cfg.Runtime.StartupReconcile)
	require.True(t, cfg.Runtime.SnapshotEnabled)
	require.Equal(t, 45*time.Second, cfg.Runtime.SnapshotTTL)
	require.Equal(t, 2*time.Minute, cfg.Runtime.IdleTimeout)
	require.True(t, cfg.Audit.AsyncEnabled)
	require.Equal(t, 128, cfg.Audit.QueueSize)
	require.True(t, cfg.Alert.AsyncEnabled)
	require.Equal(t, 16, cfg.Alert.QueueSize)
	require.True(t, cfg.History.AsyncEnabled)
	require.Equal(t, 96, cfg.History.QueueSize)
	require.True(t, cfg.Redis.Enabled)
	require.Equal(t, "127.0.0.1:6380", cfg.Redis.Addr)
	require.Equal(t, "postgres", cfg.Database.Driver)
}

func TestValidate_RejectsNegativeExecutionAndQueueSettings(t *testing.T) {
	setRequiredSecrets(t)
	cfg, err := Load("/tmp/definitely-not-exists")
	require.NoError(t, err)

	cfg.Execution.ExecutorConcurrency = -1
	require.ErrorContains(t, cfg.Validate(), "execution.executor_concurrency")

	cfg.Execution.ExecutorConcurrency = 0
	cfg.Execution.AsyncTaskQueueSize = -1
	require.ErrorContains(t, cfg.Validate(), "execution.async_task_queue_size")

	cfg.Execution.AsyncTaskQueueSize = 0
	cfg.Audit.QueueSize = -1
	require.ErrorContains(t, cfg.Validate(), "audit.queue_size")
}

func TestValidate_AllowsPostgresDriver(t *testing.T) {
	setRequiredSecrets(t)
	cfg, err := Load("/tmp/definitely-not-exists")
	require.NoError(t, err)

	cfg.Database.Driver = "postgres"
	cfg.Database.DSN = "postgres://tester:secret@127.0.0.1:5432/mcp_manager?sslmode=disable"
	require.NoError(t, cfg.Validate())
}

func TestLoad_AllowsExplicitSQLiteFallback(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("MCP_DATABASE_DRIVER", "sqlite")
	t.Setenv("MCP_DATABASE_DSN", "data/mcp_manager.db")

	cfg, err := Load("/tmp/definitely-not-exists")
	require.NoError(t, err)
	require.Equal(t, "sqlite", cfg.Database.Driver)
	require.Equal(t, "data/mcp_manager.db", cfg.Database.DSN)
}

func TestLoad_EnvOverrideNestedConfig(t *testing.T) {
	setRequiredSecrets(t)
	t.Setenv("MCP_APP_ROLE", "executor")
	t.Setenv("MCP_RPC_ENABLED", "true")
	t.Setenv("MCP_RPC_LISTEN_ADDR", "127.0.0.1:29081")
	t.Setenv("MCP_RPC_AUTH_TOKEN", "env-token")
	t.Setenv("MCP_RPC_REQUEST_TIMEOUT", "21s")

	cfg, err := Load("/tmp/definitely-not-exists")
	require.NoError(t, err)
	require.Equal(t, "executor", cfg.App.Role)
	require.True(t, cfg.RPC.Enabled)
	require.Equal(t, "127.0.0.1:29081", cfg.RPC.ListenAddr)
	require.Equal(t, "env-token", cfg.RPC.AuthToken)
	require.Equal(t, 21*time.Second, cfg.RPC.RequestTimeout)
}

func TestLoad_ControlPlaneRequiresRPCExecutorTarget(t *testing.T) {
	setRequiredSecrets(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
app:
  role: "control-plane"
rpc:
  enabled: true
  auth_token: "control-token"
  executor_target: ""
`), 0o644))

	_, err := Load(dir)
	require.ErrorContains(t, err, "rpc.executor_target 不能为空")
}

func TestLoad_ControlPlaneAllowsValidRPCConfig(t *testing.T) {
	setRequiredSecrets(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
app:
  role: "control-plane"
rpc:
  enabled: true
  executor_target: "http://127.0.0.1:19081"
  auth_token: "control-token"
  dial_timeout: 5s
  request_timeout: 12s
`), 0o644))

	cfg, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "control-plane", cfg.App.Role)
	require.True(t, cfg.RPC.Enabled)
	require.Equal(t, "http://127.0.0.1:19081", cfg.RPC.ExecutorTarget)
	require.Equal(t, "control-token", cfg.RPC.AuthToken)
	require.Equal(t, 5*time.Second, cfg.RPC.DialTimeout)
	require.Equal(t, 12*time.Second, cfg.RPC.RequestTimeout)
}

func TestLoad_ExecutorRequiresRPCListenAddr(t *testing.T) {
	setRequiredSecrets(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
app:
  role: "executor"
rpc:
  enabled: true
  listen_addr: ""
  auth_token: "executor-token"
`), 0o644))

	_, err := Load(dir)
	require.ErrorContains(t, err, "rpc.listen_addr 不能为空")
}
