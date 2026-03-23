package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/mikasa/mcp-manager/internal/handler"
	"github.com/mikasa/mcp-manager/internal/middleware"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
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

// New 创建路由
func New(jwtSvc *appcrypto.JWTService, h Handlers) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), cors())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))

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
