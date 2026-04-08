package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"syscall"
	"time"

	_ "github.com/mikasa/mcp-manager/api/docs"
	"github.com/mikasa/mcp-manager/internal/bootstrap"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/internal/service"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"go.uber.org/zap"
)

// @title MCP 服务管理平台
// @version 0.1.0
// @description MCP 服务管理平台 API
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description 输入 Bearer Token，例如 `Bearer eyJ...`
func main() {
	cfg, err := config.Load(".")
	if err != nil {
		panic(err)
	}
	if err := logger.Init(cfg.Log); err != nil {
		panic(err)
	}

	app, err := buildApp(*cfg)
	if err != nil {
		logger.L().Fatal("构建应用失败", zapError(err))
	}
	defer app.cleanup()

	if err := serveApp(app, cfg.Server.ShutdownTimeout); err != nil {
		logger.L().Fatal("运行服务失败", zapError(err))
	}
}

type managedApp struct {
	servers []namedHTTPServer
	cleanup func()
}

type namedHTTPServer struct {
	name   string
	server *http.Server
}

type serviceDisconnecter interface {
	Disconnect(ctx context.Context, serviceID string) error
}

// newHealthUpdateFn 创建健康检查状态更新回调。
func newHealthUpdateFn(serviceRepo repository.MCPServiceRepository, manager serviceDisconnecter, auditSink service.AuditSink, alertSvc service.AlertService) func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error {
	return func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error {
		item, err := serviceRepo.GetByID(ctx, serviceID)
		if err != nil {
			return err
		}
		prevStatus := item.Status
		// 先持久化最新健康状态，避免内存状态与数据库状态不一致
		if err := serviceRepo.UpdateStatus(ctx, serviceID, status, failureCount, lastError); err != nil {
			return err
		}
		if status != entity.ServiceStatusError {
			return nil
		}
		// 健康检查判定异常后主动断开连接，防止后续继续使用失效连接
		_ = manager.Disconnect(context.Background(), serviceID)
		if prevStatus == entity.ServiceStatusError {
			return nil
		}
		_ = auditSink.Record(ctx, service.AuditEntry{
			Username:     "system",
			Action:       "service_error",
			ResourceType: "mcp_service",
			ResourceID:   serviceID,
			Detail: map[string]any{
				"service_name":     item.Name,
				"transport_type":   item.TransportType,
				"status":           status,
				"failure_count":    failureCount,
				"reason":           lastError,
				"source":           "health_check",
				"listen_enabled":   item.ListenEnabled,
				"service_endpoint": endpointOf(item),
			},
		})
		_ = alertSvc.NotifyServiceError(ctx, item.Name, string(item.TransportType), endpointOf(item), lastError)
		return nil
	}
}

// buildApp 构建应用依赖并返回主程序运行时。
func buildApp(cfg config.Config) (*managedApp, error) {
	app, err := bootstrap.NewBuilder(cfg).Build()
	if err != nil {
		return nil, err
	}
	servers := collectBootstrapServers(app)
	if len(servers) == 0 {
		app.Cleanup()
		return nil, fmt.Errorf("bootstrap 未返回可启动的 HTTP 服务")
	}
	return &managedApp{servers: servers, cleanup: app.Cleanup}, nil
}

func serveApp(app *managedApp, shutdownTimeout time.Duration) error {
	if app == nil {
		return fmt.Errorf("app 不能为空")
	}
	if len(app.servers) == 0 {
		return fmt.Errorf("没有可启动的 HTTP 服务")
	}

	errCh := make(chan error, len(app.servers))
	for _, item := range app.servers {
		go func(item namedHTTPServer) {
			logger.S().Infow("server listening", "name", item.name, "addr", item.server.Addr)
			if err := item.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("%s: %w", item.name, err)
			}
		}(item)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case err := <-errCh:
		shutdownServers(app.servers, shutdownTimeout)
		return err
	case <-quit:
		shutdownServers(app.servers, shutdownTimeout)
		return nil
	}
}

func shutdownServers(servers []namedHTTPServer, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for _, item := range servers {
		if err := item.server.Shutdown(ctx); err != nil {
			logger.L().Error("关闭 HTTP 服务失败", zap.String("name", item.name), zapError(err))
		}
	}
}

func collectBootstrapServers(app *bootstrap.App) []namedHTTPServer {
	if app == nil {
		return nil
	}

	seen := map[*http.Server]struct{}{}
	servers := make([]namedHTTPServer, 0, 2)
	appendServer := func(name string, srv *http.Server) {
		if srv == nil {
			return
		}
		if _, ok := seen[srv]; ok {
			return
		}
		seen[srv] = struct{}{}
		if name == "" {
			name = fmt.Sprintf("server_%d", len(servers)+1)
		}
		servers = append(servers, namedHTTPServer{name: name, server: srv})
	}

	appendServer("http", app.Server)

	value := reflect.Indirect(reflect.ValueOf(app))
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return servers
	}

	serverType := reflect.TypeOf(&http.Server{})
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		fieldType := value.Type().Field(i)
		if fieldType.Name == "Server" {
			continue
		}
		if !field.CanInterface() {
			continue
		}
		appendReflectedServers(fieldType.Name, field.Interface(), serverType, appendServer)
	}
	return servers
}

func appendReflectedServers(name string, value any, serverType reflect.Type, appendServer func(string, *http.Server)) {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return
	}
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return
	}

	if rv.Type() == serverType {
		appendServer(name, value.(*http.Server))
		return
	}

	if rv.Kind() != reflect.Slice {
		return
	}
	for i := 0; i < rv.Len(); i++ {
		item := rv.Index(i)
		if item.Kind() == reflect.Interface && !item.IsNil() {
			item = item.Elem()
		}
		if item.Kind() == reflect.Ptr && item.Type() == serverType && !item.IsNil() {
			appendServer(fmt.Sprintf("%s_%d", name, i+1), item.Interface().(*http.Server))
		}
	}
}

// itoa 将端口等整数转换为字符串
func itoa(v int) string {
	return fmt.Sprintf("%d", v)
}

// zapError 将错误包装为 zap 字段
func zapError(err error) zap.Field {
	return zap.Error(err)
}

// endpointOf 返回服务的主要访问端点
func endpointOf(serviceItem *entity.MCPService) string {
	if serviceItem == nil {
		return ""
	}
	if serviceItem.URL != "" {
		return serviceItem.URL
	}
	return serviceItem.Command
}
