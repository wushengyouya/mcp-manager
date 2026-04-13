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

// clientAdapter 将 mcp-go 客户端适配为运行时客户端接口。
type clientAdapter struct {
	inner *mcpgoclient.Client
}

// Start 启动底层 MCP 客户端连接。
func (a *clientAdapter) Start(ctx context.Context) error {
	return a.inner.Start(ctx)
}

// Initialize 发起 MCP 初始化握手。
func (a *clientAdapter) Initialize(ctx context.Context, request mcp.InitializeRequest) (*mcp.InitializeResult, error) {
	return a.inner.Initialize(ctx, request)
}

// Close 关闭底层 MCP 客户端连接。
func (a *clientAdapter) Close() error {
	return a.inner.Close()
}

// ListTools 获取远端服务暴露的工具列表。
func (a *clientAdapter) ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	return a.inner.ListTools(ctx, request)
}

// CallTool 调用远端服务上的指定工具。
func (a *clientAdapter) CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return a.inner.CallTool(ctx, request)
}

// Ping 向远端服务发送心跳请求。
func (a *clientAdapter) Ping(ctx context.Context) error {
	return a.inner.Ping(ctx)
}

// GetSessionId 返回当前连接持有的会话 ID。
func (a *clientAdapter) GetSessionId() string {
	return a.inner.GetSessionId()
}

// OnNotification 注册服务端通知回调。
func (a *clientAdapter) OnNotification(handler func(notification mcp.JSONRPCNotification)) {
	a.inner.OnNotification(handler)
}

// OnConnectionLost 注册连接丢失回调。
func (a *clientAdapter) OnConnectionLost(handler func(error)) {
	a.inner.OnConnectionLost(handler)
}

var (
	newStdioClient = func(command string, env []string, args []string) (runtimeClient, error) {
		cli, err := mcpgoclient.NewStdioMCPClientWithOptions(command, env, args, transport.WithCommandLogger(logger.S()))
		if err != nil {
			return nil, err
		}
		return &clientAdapter{inner: cli}, nil
	}
	newSSEClient = func(url string, headers map[string]string, httpClient *http.Client) (runtimeClient, error) {
		cli, err := mcpgoclient.NewSSEMCPClient(url, mcpgoclient.WithHeaders(headers), mcpgoclient.WithHTTPClient(httpClient))
		if err != nil {
			return nil, err
		}
		return &clientAdapter{inner: cli}, nil
	}
	newStreamableHTTPClient = func(url string, opts ...transport.StreamableHTTPCOption) (runtimeClient, error) {
		cli, err := mcpgoclient.NewStreamableHttpClient(url, opts...)
		if err != nil {
			return nil, err
		}
		return &clientAdapter{inner: cli}, nil
	}
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
	mc.bindRuntimeCallbacks(mc.client)

	if err := mc.initialize(appCfg); err != nil {
		_ = mc.close()
		return nil, err
	}
	return mc, nil
}

// buildClient 按服务传输类型构建底层 MCP 客户端
func buildClient(service *entity.MCPService) (runtimeClient, entity.TransportType, error) {
	timeout := time.Duration(service.Timeout) * time.Second
	headers := buildHeaders(service)

	switch service.TransportType {
	case entity.TransportTypeStdio:
		env := make([]string, 0, len(service.Env))
		for k, v := range service.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cli, err := newStdioClient(service.Command, env, []string(service.Args))
		return cli, entity.TransportTypeStdio, err
	case entity.TransportTypeSSE:
		httpClient := &http.Client{Timeout: timeout}
		cli, err := newSSEClient(service.URL, headers, httpClient)
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
		cli, err := newStreamableHTTPClient(service.URL, opts...)
		return cli, entity.TransportTypeStreamableHTTP, err
	default:
		return nil, "", fmt.Errorf("unsupported transport: %s", service.TransportType)
	}
}

// initialize 完成客户端启动和 MCP 初始化握手
func (m *managedClient) initialize(appCfg config.AppConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.service.Timeout)*time.Second)
	defer cancel()
	primary := m.activeClient()
	actualTransport := m.actualTransport

	if err := primary.Start(ctx); err != nil {
		return err
	}

	// 优先使用声明的传输方式初始化，兼容模式开启时才退回 legacy SSE
	result, err := primary.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    appCfg.Name,
				Version: appCfg.Version,
			},
		},
	})
	if err != nil && m.service.TransportType == entity.TransportTypeStreamableHTTP && m.service.CompatMode == "allow_legacy_sse" {
		fallback, fallbackErr := newSSEClient(m.service.URL, buildHeaders(m.service), &http.Client{Timeout: time.Duration(m.service.Timeout) * time.Second})
		if fallbackErr != nil {
			return err
		}
		m.bindRuntimeCallbacks(fallback)
		if startErr := fallback.Start(ctx); startErr != nil {
			_ = fallback.Close()
			return startErr
		}
		fallbackResult, fallbackInitErr := fallback.Initialize(ctx, mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: appCfg.Name, Version: appCfg.Version},
			},
		})
		if fallbackInitErr != nil {
			_ = fallback.Close()
			err = fallbackInitErr
		} else {
			// 切换活跃客户端前先让旧连接失活，避免其延迟回调覆盖新运行态。
			m.setActiveClient(nil)
			if closeErr := m.closeClient(primary); closeErr != nil {
				m.setActiveClient(primary)
				_ = fallback.Close()
				return closeErr
			}
			m.setActiveClient(fallback)
			actualTransport = entity.TransportTypeSSE
			result = fallbackResult
			err = nil
		}
	}
	if err != nil {
		return err
	}

	now := time.Now()
	activeClient := m.activeClient()
	sessionExists := activeClient != nil && activeClient.GetSessionId() != ""
	m.mu.Lock()
	m.actualTransport = actualTransport
	m.runtime.Status = entity.ServiceStatusConnected
	m.runtime.ConnectedAt = &now
	m.runtime.ProtocolVersion = result.ProtocolVersion
	// 保留初始化返回的能力快照，供状态查询接口直接复用
	m.runtime.TransportCapabilities = map[string]any{
		"tools":    result.Capabilities.Tools != nil,
		"logging":  result.Capabilities.Logging != nil,
		"prompts":  result.Capabilities.Prompts != nil,
		"roots":    result.Capabilities.Roots != nil,
		"sampling": result.Capabilities.Sampling != nil,
	}
	m.runtime.SessionIDExists = sessionExists
	m.runtime.LastSeenAt = &now
	m.runtime.ListenActive = m.service.ListenEnabled
	m.runtime.TransportType = string(actualTransport)
	m.mu.Unlock()
	if err := validateSessionMode(m.service.TransportType, actualTransport, m.service.SessionMode, sessionExists); err != nil {
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

// bindRuntimeCallbacks 绑定运行态更新回调，并忽略来自旧客户端的迟到事件。
func (m *managedClient) bindRuntimeCallbacks(cli runtimeClient) {
	cli.OnNotification(func(notification mcp.JSONRPCNotification) {
		// 收到服务端通知即可视为监听链路仍然可用。
		now := time.Now()
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.client != cli {
			return
		}
		m.runtime.LastSeenAt = &now
		m.runtime.ListenActive = true
		m.runtime.ListenLastError = ""
	})
	cli.OnConnectionLost(func(err error) {
		// 底层连接丢失后立即刷新运行态，避免继续暴露为健康连接。
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.client != cli {
			return
		}
		m.runtime.Status = entity.ServiceStatusError
		m.runtime.LastError = err.Error()
		m.runtime.ListenLastError = err.Error()
		m.runtime.ListenActive = false
	})
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
		if cli := m.activeClient(); cli != nil {
			err = m.closeClient(cli)
		}
	})
	return err
}

// activeClient 返回当前处于活跃状态的底层客户端。
func (m *managedClient) activeClient() runtimeClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client
}

// setActiveClient 切换当前活跃的底层客户端引用。
func (m *managedClient) setActiveClient(cli runtimeClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.client = cli
}

// closeClient 关闭指定的底层客户端连接。
func (m *managedClient) closeClient(cli runtimeClient) error {
	return cli.Close()
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

// beginInteraction 标记一次真实业务交互开始。
func (m *managedClient) beginInteraction() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime.InFlight++
}

// endInteraction 标记一次真实业务交互结束，并刷新最近使用时间。
func (m *managedClient) endInteraction() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.runtime.InFlight > 0 {
		m.runtime.InFlight--
	}
	m.runtime.LastUsedAt = &now
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
