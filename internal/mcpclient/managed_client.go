package mcpclient

import (
	"context"
	"fmt"
	"net/http"
	"time"

	mcpgoclient "github.com/mark3labs/mcp-go/client"
	transport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/pkg/logger"
)

// newManagedClient 创建并初始化受管 MCP 客户端
func newManagedClient(appCfg config.AppConfig, service *entity.MCPService) (*managedClient, error) {
	mc := &managedClient{
		service: service,
		runtime: RuntimeStatus{
			ServiceID:             service.ID,
			Status:                entity.ServiceStatusConnecting,
			TransportType:         string(service.TransportType),
			ListenEnabled:         service.ListenEnabled,
			TransportCapabilities: map[string]any{},
			LastError:             service.LastError,
			FailureCount:          service.FailureCount,
		},
		actualTransport: service.TransportType,
	}

	cli, actualTransport, err := buildClient(service)
	if err != nil {
		return nil, err
	}
	mc.client = cli
	mc.actualTransport = actualTransport
	mc.runtime.TransportType = string(actualTransport)
	mc.client.OnNotification(func(notification mcp.JSONRPCNotification) {
		// 收到服务端通知即可视为监听链路仍然可用
		now := time.Now()
		mc.mu.Lock()
		mc.runtime.LastSeenAt = &now
		mc.runtime.ListenActive = true
		mc.runtime.ListenLastError = ""
		mc.mu.Unlock()
	})
	mc.client.OnConnectionLost(func(err error) {
		// 底层连接丢失后立即刷新运行态，避免继续暴露为健康连接
		mc.mu.Lock()
		defer mc.mu.Unlock()
		mc.runtime.Status = entity.ServiceStatusError
		mc.runtime.LastError = err.Error()
		mc.runtime.ListenLastError = err.Error()
		mc.runtime.ListenActive = false
	})

	if err := mc.initialize(appCfg); err != nil {
		_ = mc.close()
		return nil, err
	}
	return mc, nil
}

// buildClient 按服务传输类型构建底层 MCP 客户端
func buildClient(service *entity.MCPService) (*mcpgoclient.Client, entity.TransportType, error) {
	timeout := time.Duration(service.Timeout) * time.Second
	headers := buildHeaders(service)

	switch service.TransportType {
	case entity.TransportTypeStdio:
		env := make([]string, 0, len(service.Env))
		for k, v := range service.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cli, err := mcpgoclient.NewStdioMCPClientWithOptions(service.Command, env, []string(service.Args), transport.WithCommandLogger(logger.S()))
		return cli, entity.TransportTypeStdio, err
	case entity.TransportTypeSSE:
		httpClient := &http.Client{Timeout: timeout}
		cli, err := mcpgoclient.NewSSEMCPClient(service.URL, mcpgoclient.WithHeaders(headers), mcpgoclient.WithHTTPClient(httpClient))
		return cli, entity.TransportTypeSSE, err
	case entity.TransportTypeStreamableHTTP:
		opts := []transport.StreamableHTTPCOption{
			transport.WithHTTPHeaders(headers),
			transport.WithHTTPTimeout(timeout),
			transport.WithHTTPLogger(logger.S()),
		}
		if service.ListenEnabled {
			opts = append(opts, transport.WithContinuousListening())
		}
		cli, err := mcpgoclient.NewStreamableHttpClient(service.URL, opts...)
		return cli, entity.TransportTypeStreamableHTTP, err
	default:
		return nil, "", fmt.Errorf("unsupported transport: %s", service.TransportType)
	}
}

// initialize 完成客户端启动和 MCP 初始化握手
func (m *managedClient) initialize(appCfg config.AppConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.service.Timeout)*time.Second)
	defer cancel()

	if err := m.client.Start(ctx); err != nil {
		return err
	}

	// 优先使用声明的传输方式初始化，兼容模式开启时才退回 legacy SSE
	result, err := m.client.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    appCfg.Name,
				Version: appCfg.Version,
			},
		},
	})
	if err != nil && m.service.TransportType == entity.TransportTypeStreamableHTTP && m.service.CompatMode == "allow_legacy_sse" {
		fallback, fallbackErr := mcpgoclient.NewSSEMCPClient(m.service.URL, mcpgoclient.WithHeaders(buildHeaders(m.service)), mcpgoclient.WithHTTPClient(&http.Client{Timeout: time.Duration(m.service.Timeout) * time.Second}))
		if fallbackErr != nil {
			return err
		}
		m.client = fallback
		m.actualTransport = entity.TransportTypeSSE
		if startErr := m.client.Start(ctx); startErr != nil {
			return err
		}
		result, err = m.client.Initialize(ctx, mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: appCfg.Name, Version: appCfg.Version},
			},
		})
	}
	if err != nil {
		return err
	}

	now := time.Now()
	m.mu.Lock()
	m.runtime.Status = entity.ServiceStatusConnected
	m.runtime.ProtocolVersion = result.ProtocolVersion
	// 保留初始化返回的能力快照，供状态查询接口直接复用
	m.runtime.TransportCapabilities = map[string]any{
		"tools":    result.Capabilities.Tools != nil,
		"logging":  result.Capabilities.Logging != nil,
		"prompts":  result.Capabilities.Prompts != nil,
		"roots":    result.Capabilities.Roots != nil,
		"sampling": result.Capabilities.Sampling != nil,
	}
	m.runtime.SessionIDExists = m.client.GetSessionId() != ""
	m.runtime.LastSeenAt = &now
	m.runtime.ListenActive = m.service.ListenEnabled
	m.runtime.TransportType = string(m.actualTransport)
	m.mu.Unlock()
	if err := validateSessionMode(m.service.TransportType, m.actualTransport, m.service.SessionMode, m.client.GetSessionId() != ""); err != nil {
		m.markTerminalError(err)
		return err
	}
	return nil
}

// buildHeaders 组装远程服务请求头
func buildHeaders(service *entity.MCPService) map[string]string {
	headers := map[string]string{
		"User-Agent": "mcp-manager",
	}
	for k, v := range service.CustomHeaders {
		headers[k] = v
	}
	if service.BearerToken != "" {
		headers["Authorization"] = "Bearer " + service.BearerToken
	}
	return headers
}

// runtimeStatus 返回当前运行态的副本
func (m *managedClient) runtimeStatus() RuntimeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := m.runtime
	out.SessionIDExists = m.client != nil && m.client.GetSessionId() != ""
	return out
}

// close 幂等关闭底层客户端连接
func (m *managedClient) close() error {
	var err error
	m.closeOnce.Do(func() {
		if m.client != nil {
			err = m.client.Close()
		}
	})
	return err
}

// markError 在调用失败时记录最新错误状态
func (m *managedClient) markError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime.LastError = err.Error()
	m.runtime.ListenLastError = err.Error()
	m.runtime.ListenActive = false
}

// markTerminalError 记录需要人工重新建立连接的终止性错误。
func (m *managedClient) markTerminalError(err error) RuntimeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime.Status = entity.ServiceStatusError
	m.runtime.LastError = err.Error()
	m.runtime.ListenLastError = err.Error()
	m.runtime.ListenActive = false
	m.runtime.SessionIDExists = false
	return m.runtime
}

// markSeen 在成功交互后刷新连接健康信息
func (m *managedClient) markSeen() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime.Status = entity.ServiceStatusConnected
	m.runtime.FailureCount = 0
	m.runtime.LastSeenAt = &now
	m.runtime.LastError = ""
	m.runtime.ListenActive = m.service.ListenEnabled
	m.runtime.ListenLastError = ""
}

// applyHealthState 应用健康检查计算出的状态
func (m *managedClient) applyHealthState(status entity.ServiceStatus, failureCount int, lastError string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime.Status = status
	m.runtime.FailureCount = failureCount
	m.runtime.LastError = lastError
	if lastError != "" {
		m.runtime.ListenLastError = lastError
		m.runtime.ListenActive = false
		return
	}
	m.runtime.ListenLastError = ""
	m.runtime.ListenActive = m.service.ListenEnabled
}
