package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 定义应用完整配置
type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	Database    DatabaseConfig    `mapstructure:"database"`
	JWT         JWTConfig         `mapstructure:"jwt"`
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
	StatusSource     string `mapstructure:"status_source"`
	StartupReconcile bool   `mapstructure:"startup_reconcile"`
}

// HistoryConfig 定义调用历史治理配置
type HistoryConfig struct {
	MaxBodyBytes int    `mapstructure:"max_body_bytes"`
	Compression  string `mapstructure:"compression"`
}

// Load 加载应用配置
func Load(paths ...string) (*Config, error) {
	v := viper.New()
	setDefaults(v)
	v.SetEnvPrefix("MCP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

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

// setDefaults 注入系统默认配置值
func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "release")
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.shutdown_timeout", "15s")
	v.SetDefault("database.driver", "sqlite")
	v.SetDefault("database.dsn", "data/mcp_manager.db")
	v.SetDefault("database.max_open_conns", 1)
	v.SetDefault("database.max_idle_conns", 1)
	v.SetDefault("database.conn_max_lifetime", "1h")
	v.SetDefault("jwt.issuer", "mcp-manager")
	v.SetDefault("jwt.access_ttl", "2h")
	v.SetDefault("jwt.refresh_ttl", "168h")
	v.SetDefault("health_check.enabled", true)
	v.SetDefault("health_check.interval", "30s")
	v.SetDefault("health_check.timeout", "10s")
	v.SetDefault("health_check.failure_threshold", 3)
	v.SetDefault("audit.retention_days", 90)
	v.SetDefault("audit.cleanup_interval", "24h")
	v.SetDefault("alert.smtp_port", 587)
	v.SetDefault("alert.subject_prefix", "[MCP-MANAGER]")
	v.SetDefault("alert.silence_window", "30m")
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
	v.SetDefault("history.max_body_bytes", 8192)
	v.SetDefault("history.compression", "none")
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
	if c.HealthCheck.FailureThreshold <= 0 {
		return fmt.Errorf("health_check.failure_threshold 必须大于 0")
	}
	switch c.Database.Driver {
	case "sqlite", "postgres":
	default:
		return fmt.Errorf("database.driver 仅支持 sqlite 或 postgres")
	}
	return nil
}
