package router

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/mikasa/mcp-manager/internal/handler"
	"github.com/mikasa/mcp-manager/internal/middleware"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
)

const (
	RoleAll          = "all"
	RoleControlPlane = "control-plane"
	RoleExecutor     = "executor"
)

// Handlers 聚合所有处理器
type Handlers struct {
	Auth    *handler.AuthHandler
	User    *handler.UserHandler
	MCP     *handler.MCPHandler
	Tool    *handler.ToolHandler
	History *handler.HistoryHandler
	Audit   *handler.AuditHandler
}

// ReadinessCheck 描述单项 readiness 检查结果。
type ReadinessCheck struct {
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
}

// ReadinessStatus 描述 readiness 响应。
type ReadinessStatus struct {
	Role   string                    `json:"role"`
	Ready  bool                      `json:"ready"`
	Checks map[string]ReadinessCheck `json:"checks"`
	Reason string                    `json:"reason,omitempty"`
}

// ReadinessProbe 定义 readiness 注入探针。
type ReadinessProbe func(ctx context.Context) ReadinessStatus

type options struct {
	role           string
	readinessProbe ReadinessProbe
}

// Option 定义路由构造选项。
type Option func(*options)

// WithRole 指定当前进程角色。
func WithRole(role string) Option {
	return func(opts *options) {
		opts.role = normalizeRole(role)
	}
}

// WithReadinessProbe 注入 role-aware readiness 探针。
func WithReadinessProbe(probe ReadinessProbe) Option {
	return func(opts *options) {
		opts.readinessProbe = probe
	}
}

// New 创建路由
func New(jwtSvc *appcrypto.JWTService, h Handlers, opts ...Option) *gin.Engine {
	cfg := buildOptions(opts...)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), cors())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/ready", readinessHandler(cfg))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))

	if cfg.role == RoleExecutor {
		return r
	}

	api := r.Group("/api/v1")
	{
		api.POST("/auth/login", h.Auth.Login)
		api.POST("/auth/refresh", h.Auth.Refresh)
	}

	auth := api.Group("")
	auth.Use(middleware.Auth(jwtSvc))
	{
		auth.POST("/auth/logout", h.Auth.Logout)
		auth.GET("/services", h.MCP.List)
		auth.GET("/services/:id", h.MCP.Get)
		auth.GET("/services/:id/status", h.MCP.Status)
		auth.GET("/services/:id/tools", h.Tool.ListByService)
		auth.GET("/tools/:id", h.Tool.Get)
		auth.GET("/history", h.History.List)
		auth.GET("/history/:id", h.History.Get)
		auth.PUT("/users/:id/password", h.User.ChangePassword)
	}

	modify := auth.Group("")
	modify.Use(middleware.RequireModify())
	{
		modify.POST("/services", h.MCP.Create)
		modify.PUT("/services/:id", h.MCP.Update)
		modify.DELETE("/services/:id", h.MCP.Delete)
		modify.POST("/services/:id/connect", h.MCP.Connect)
		modify.POST("/services/:id/disconnect", h.MCP.Disconnect)
		modify.POST("/services/:id/sync-tools", h.Tool.Sync)
		modify.POST("/tools/:id/invoke", h.Tool.Invoke)
	}

	admin := auth.Group("")
	admin.Use(middleware.RequireAdmin())
	{
		admin.GET("/users", h.User.List)
		admin.POST("/users", h.User.Create)
		admin.PUT("/users/:id", h.User.Update)
		admin.DELETE("/users/:id", h.User.Delete)
		admin.GET("/audit-logs", h.Audit.List)
		admin.GET("/audit-logs/export", h.Audit.Export)
	}
	return r
}

// cors 返回基础跨域中间件
func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func buildOptions(appliers ...Option) options {
	opts := options{role: RoleAll}
	for _, apply := range appliers {
		if apply != nil {
			apply(&opts)
		}
	}
	opts.role = normalizeRole(opts.role)
	return opts
}

func normalizeRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case RoleControlPlane:
		return RoleControlPlane
	case RoleExecutor:
		return RoleExecutor
	default:
		return RoleAll
	}
}

func readinessHandler(opts options) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := defaultReadinessStatus(opts.role)
		if opts.readinessProbe != nil {
			status = mergeReadinessStatus(status, opts.readinessProbe(c.Request.Context()))
		}
		code := http.StatusOK
		if !status.Ready {
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, status)
	}
}

func defaultReadinessStatus(role string) ReadinessStatus {
	return ReadinessStatus{
		Role:  normalizeRole(role),
		Ready: true,
		Checks: map[string]ReadinessCheck{
			"http": {Ready: true},
		},
		Reason: "router initialized",
	}
}

func mergeReadinessStatus(base, override ReadinessStatus) ReadinessStatus {
	if override.Role != "" {
		base.Role = normalizeRole(override.Role)
	}
	base.Ready = override.Ready
	if override.Reason != "" {
		base.Reason = override.Reason
	}
	if len(override.Checks) > 0 {
		checks := make(map[string]ReadinessCheck, len(base.Checks)+len(override.Checks))
		for name, check := range base.Checks {
			checks[name] = check
		}
		for name, check := range override.Checks {
			checks[name] = check
		}
		base.Checks = checks
	}
	if base.Checks == nil {
		base.Checks = map[string]ReadinessCheck{}
	}
	if _, ok := base.Checks["http"]; !ok {
		base.Checks["http"] = ReadinessCheck{Ready: true}
	}
	return base
}
