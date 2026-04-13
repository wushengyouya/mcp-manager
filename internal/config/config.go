package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 定义应用完整配置
type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Database    DatabaseConfig    `mapstructure:"database"`
	JWT         JWTConfig         `mapstructure:"jwt"`
	Redis       RedisConfig       `mapstructure:"redis"`
	RPC         RPCConfig         `mapstructure:"rpc"`
	Execution   ExecutionConfig   `mapstructure:"execution"`
	HealthCheck HealthCheckConfig `mapstructure:"health_check"`
	Audit       AuditConfig       `mapstructure:"audit"`
	Alert       AlertConfig       `mapstructure:"alert"`
	Log         LogConfig         `mapstructure:"log"`
	App         AppConfig         `mapstructure:"app"`
	Runtime     RuntimeConfig     `mapstructure:"runtime"`
	History     HistoryConfig     `mapstructure:"history"`
}

// ServerConfig 定义 HTTP 服务配置
type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Mode            string        `mapstructure:"mode"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// DatabaseConfig 定义数据库配置
type DatabaseConfig struct {
	Driver          string        `mapstructure:"driver"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// JWTConfig 定义 JWT 配置
type JWTConfig struct {
	Issuer     string        `mapstructure:"issuer"`
	Secret     string        `mapstructure:"secret"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

// RedisConfig 定义 Redis 配置。
type RedisConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Addr             string        `mapstructure:"addr"`
	Password         string        `mapstructure:"password"`
	DB               int           `mapstructure:"db"`
	KeyPrefix        string        `mapstructure:"key_prefix"`
	DialTimeout      time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout      time.Duration `mapstructure:"read_timeout"`
	WriteTimeout     time.Duration `mapstructure:"write_timeout"`
	OperationTimeout time.Duration `mapstructure:"operation_timeout"`
}

// RPCConfig 定义内部 RPC 配置。
type RPCConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	ListenAddr     string        `mapstructure:"listen_addr"`
	ExecutorTarget string        `mapstructure:"executor_target"`
	AuthToken      string        `mapstructure:"auth_token"`
	DialTimeout    time.Duration `mapstructure:"dial_timeout"`
	RequestTimeout time.Duration `mapstructure:"request_timeout"`
}

// ExecutionConfig 定义执行治理配置。
type ExecutionConfig struct {
	ExecutorConcurrency int           `mapstructure:"executor_concurrency"`
	ServiceRateLimit    int           `mapstructure:"service_rate_limit"`
	UserRateLimit       int           `mapstructure:"user_rate_limit"`
	RateLimitWindow     time.Duration `mapstructure:"rate_limit_window"`
	AsyncInvokeEnabled  bool          `mapstructure:"async_invoke_enabled"`
	AsyncTaskQueueSize  int           `mapstructure:"async_task_queue_size"`
	AsyncTaskWorkers    int           `mapstructure:"async_task_workers"`
	DefaultTaskTimeout  time.Duration `mapstructure:"default_task_timeout"`
}

// HealthCheckConfig 定义健康检查配置
type HealthCheckConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Interval         time.Duration `mapstructure:"interval"`
	Timeout          time.Duration `mapstructure:"timeout"`
	FailureThreshold int           `mapstructure:"failure_threshold"`
}

// AuditConfig 定义审计配置
type AuditConfig struct {
	RetentionDays   int           `mapstructure:"retention_days"`
	CleanupInterval time.Duration `mapstructure:"cleanup_interval"`
	AsyncEnabled    bool          `mapstructure:"async_enabled"`
	QueueSize       int           `mapstructure:"queue_size"`
}

// AlertConfig 定义告警配置
type AlertConfig struct {
	Enabled       bool          `mapstructure:"enabled"`
	From          string        `mapstructure:"from"`
	To            []string      `mapstructure:"to"`
	SMTPHost      string        `mapstructure:"smtp_host"`
	SMTPPort      int           `mapstructure:"smtp_port"`
	SMTPUsername  string        `mapstructure:"smtp_username"`
	SMTPPassword  string        `mapstructure:"smtp_password"`
	SubjectPrefix string        `mapstructure:"subject_prefix"`
	SilenceWindow time.Duration `mapstructure:"silence_window"`
	AsyncEnabled  bool          `mapstructure:"async_enabled"`
	QueueSize     int           `mapstructure:"queue_size"`
}

// LogConfig 定义日志配置
type LogConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	Output     string `mapstructure:"output"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"`
	Compress   bool   `mapstructure:"compress"`
}

// AppConfig 定义应用元信息和初始化配置
type AppConfig struct {
	Name              string `mapstructure:"name"`
	Version           string `mapstructure:"version"`
	Role              string `mapstructure:"role"`
	InitAdminUsername string `mapstructure:"init_admin_username"`
	InitAdminPassword string `mapstructure:"init_admin_password"`
	InitAdminEmail    string `mapstructure:"init_admin_email"`
}

// RuntimeConfig 定义运行态占位配置。
type RuntimeConfig struct {
	StatusSource                  string        `mapstructure:"status_source"`
	StartupReconcile              bool          `mapstructure:"startup_reconcile"`
	SnapshotEnabled               bool          `mapstructure:"snapshot_enabled"`
	SnapshotTTL                   time.Duration `mapstructure:"snapshot_ttl"`
	IdleTimeout                   time.Duration `mapstructure:"idle_timeout"`
	IdleReaperDryRunEnabled       bool          `mapstructure:"idle_reaper_dry_run_enabled"`
	OwnerConflictDetectionEnabled bool          `mapstructure:"owner_conflict_detection_enabled"`
}

// HistoryConfig 定义调用历史治理配置
type HistoryConfig struct {
	MaxBodyBytes int    `mapstructure:"max_body_bytes"`
	Compression  string `mapstructure:"compression"`
	AsyncEnabled bool   `mapstructure:"async_enabled"`
	QueueSize    int    `mapstructure:"queue_size"`
}

// Load 加载应用配置
func Load(paths ...string) (*Config, error) {
	v := viper.New()
	setDefaults(v)
	v.SetEnvPrefix("MCP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	if err := bindEnvKeys(v, "", reflect.TypeOf(Config{})); err != nil {
		return nil, fmt.Errorf("绑定环境变量失败: %w", err)
	}

	if len(paths) == 0 {
		paths = []string{".", "./deployments/docker"}
	}
	for _, path := range paths {
		v.AddConfigPath(path)
	}
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !os.IsNotExist(err) && !isConfigNotFound(err, &notFound) {
			return nil, fmt.Errorf("读取配置失败: %w", err)
		}
	}

	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	if secret := os.Getenv("MCP_JWT_SECRET"); secret != "" {
		cfg.JWT.Secret = secret
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// isConfigNotFound 判断错误是否为配置文件缺失
func isConfigNotFound(err error, target *viper.ConfigFileNotFoundError) bool {
	_, ok := err.(viper.ConfigFileNotFoundError)
	return ok
}

func bindEnvKeys(v *viper.Viper, prefix string, t reflect.Type) error {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	durationType := reflect.TypeOf(time.Duration(0))
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("mapstructure")
		if idx := strings.Index(name, ","); idx >= 0 {
			name = name[:idx]
		}
		if name == "" || name == "-" {
			continue
		}

		key := name
		if prefix != "" {
			key = prefix + "." + name
		}

		fieldType := field.Type
		if fieldType.Kind() == reflect.Struct && fieldType != durationType {
			if err := bindEnvKeys(v, key, fieldType); err != nil {
				return err
			}
			continue
		}

		if err := v.BindEnv(key); err != nil {
			return err
		}
	}

	return nil
}

// setDefaults 注入系统默认配置值
func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "release")
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "15s")
	v.SetDefault("database.driver", "postgres")
	v.SetDefault("database.dsn", "postgres://postgres:postgres@127.0.0.1:5432/mcp_manager?sslmode=disable")
	v.SetDefault("database.max_open_conns", 1)
	v.SetDefault("database.max_idle_conns", 1)
	v.SetDefault("database.conn_max_lifetime", "1h")
	v.SetDefault("jwt.issuer", "mcp-manager")
	v.SetDefault("jwt.access_ttl", "2h")
	v.SetDefault("jwt.refresh_ttl", "168h")
	v.SetDefault("redis.enabled", true)
	v.SetDefault("redis.addr", "127.0.0.1:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.key_prefix", "mcp-manager:")
	v.SetDefault("redis.dial_timeout", "3s")
	v.SetDefault("redis.read_timeout", "2s")
	v.SetDefault("redis.write_timeout", "2s")
	v.SetDefault("redis.operation_timeout", "2s")
	v.SetDefault("rpc.enabled", false)
	v.SetDefault("rpc.listen_addr", "127.0.0.1:18081")
	v.SetDefault("rpc.executor_target", "http://127.0.0.1:18081")
	v.SetDefault("rpc.auth_token", "dev-only-rpc-token")
	v.SetDefault("rpc.dial_timeout", "3s")
	v.SetDefault("rpc.request_timeout", "10s")
	v.SetDefault("execution.executor_concurrency", 0)
	v.SetDefault("execution.service_rate_limit", 0)
	v.SetDefault("execution.user_rate_limit", 0)
	v.SetDefault("execution.rate_limit_window", "1m")
	v.SetDefault("execution.async_invoke_enabled", false)
	v.SetDefault("execution.async_task_queue_size", 64)
	v.SetDefault("execution.async_task_workers", 2)
	v.SetDefault("execution.default_task_timeout", "30s")
	v.SetDefault("health_check.enabled", true)
	v.SetDefault("health_check.interval", "30s")
	v.SetDefault("health_check.timeout", "10s")
	v.SetDefault("health_check.failure_threshold", 3)
	v.SetDefault("audit.retention_days", 90)
	v.SetDefault("audit.cleanup_interval", "24h")
	v.SetDefault("audit.async_enabled", false)
	v.SetDefault("audit.queue_size", 256)
	v.SetDefault("alert.smtp_port", 587)
	v.SetDefault("alert.subject_prefix", "[MCP-MANAGER]")
	v.SetDefault("alert.silence_window", "30m")
	v.SetDefault("alert.async_enabled", false)
	v.SetDefault("alert.queue_size", 64)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "console")
	v.SetDefault("log.output", "stdout")
	v.SetDefault("log.max_size", 50)
	v.SetDefault("log.max_backups", 5)
	v.SetDefault("log.max_age", 30)
	v.SetDefault("app.name", "mcp-manager")
	v.SetDefault("app.version", "0.1.0")
	v.SetDefault("app.role", "all")
	v.SetDefault("app.init_admin_username", "root")
	v.SetDefault("app.init_admin_password", "admin123456")
	v.SetDefault("app.init_admin_email", "root@example.com")
	v.SetDefault("runtime.status_source", "runtime_first")
	v.SetDefault("runtime.startup_reconcile", true)
	v.SetDefault("runtime.snapshot_enabled", false)
	v.SetDefault("runtime.snapshot_ttl", "30s")
	v.SetDefault("runtime.idle_timeout", "0s")
	v.SetDefault("runtime.idle_reaper_dry_run_enabled", false)
	v.SetDefault("runtime.owner_conflict_detection_enabled", false)
	v.SetDefault("history.max_body_bytes", 8192)
	v.SetDefault("history.compression", "none")
	v.SetDefault("history.async_enabled", false)
	v.SetDefault("history.queue_size", 256)
}

// Validate 校验配置合法性
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port 非法")
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log.level 非法")
	}
	switch c.Log.Format {
	case "json", "console":
	default:
		return fmt.Errorf("log.format 非法")
	}
	switch c.App.Role {
	case "all", "control-plane", "executor":
	default:
		return fmt.Errorf("app.role 非法")
	}
	if c.JWT.Secret == "" {
		c.JWT.Secret = "dev-only-secret-change-me"
	}
	if err := c.validateRPC(); err != nil {
		return err
	}
	if c.HealthCheck.FailureThreshold <= 0 {
		return fmt.Errorf("health_check.failure_threshold 必须大于 0")
	}
	switch c.Database.Driver {
	case "sqlite", "postgres":
	default:
		return fmt.Errorf("database.driver 仅支持 sqlite 或 postgres")
	}
	if c.Redis.Enabled && strings.TrimSpace(c.Redis.Addr) == "" {
		return fmt.Errorf("redis.addr 不能为空")
	}
	if c.Execution.ExecutorConcurrency < 0 {
		return fmt.Errorf("execution.executor_concurrency 不能小于 0")
	}
	if c.Execution.ServiceRateLimit < 0 {
		return fmt.Errorf("execution.service_rate_limit 不能小于 0")
	}
	if c.Execution.UserRateLimit < 0 {
		return fmt.Errorf("execution.user_rate_limit 不能小于 0")
	}
	if c.Execution.RateLimitWindow <= 0 {
		return fmt.Errorf("execution.rate_limit_window 必须大于 0")
	}
	if c.Execution.AsyncTaskQueueSize < 0 {
		return fmt.Errorf("execution.async_task_queue_size 不能小于 0")
	}
	if c.Execution.AsyncTaskWorkers < 0 {
		return fmt.Errorf("execution.async_task_workers 不能小于 0")
	}
	if c.Execution.DefaultTaskTimeout < 0 {
		return fmt.Errorf("execution.default_task_timeout 不能小于 0")
	}
	if c.Runtime.SnapshotTTL < 0 {
		return fmt.Errorf("runtime.snapshot_ttl 不能小于 0")
	}
	if c.Runtime.IdleTimeout < 0 {
		return fmt.Errorf("runtime.idle_timeout 不能小于 0")
	}
	if c.Audit.QueueSize < 0 {
		return fmt.Errorf("audit.queue_size 不能小于 0")
	}
	if c.Alert.QueueSize < 0 {
		return fmt.Errorf("alert.queue_size 不能小于 0")
	}
	if c.History.QueueSize < 0 {
		return fmt.Errorf("history.queue_size 不能小于 0")
	}
	return nil
}

func (c *Config) validateRPC() error {
	if !c.RPC.Enabled {
		switch c.App.Role {
		case "control-plane", "executor":
			return fmt.Errorf("rpc.enabled 在 %s 模式下必须为 true", c.App.Role)
		default:
			return nil
		}
	}
	if strings.TrimSpace(c.RPC.AuthToken) == "" {
		return fmt.Errorf("rpc.auth_token 不能为空")
	}
	if c.RPC.DialTimeout <= 0 {
		return fmt.Errorf("rpc.dial_timeout 必须大于 0")
	}
	if c.RPC.RequestTimeout <= 0 {
		return fmt.Errorf("rpc.request_timeout 必须大于 0")
	}
	switch c.App.Role {
	case "control-plane":
		if strings.TrimSpace(c.RPC.ExecutorTarget) == "" {
			return fmt.Errorf("rpc.executor_target 不能为空")
		}
	case "executor":
		if strings.TrimSpace(c.RPC.ListenAddr) == "" {
			return fmt.Errorf("rpc.listen_addr 不能为空")
		}
	}
	return nil
}
