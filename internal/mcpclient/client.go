package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	mcpgoclient "github.com/mark3labs/mcp-go/client"
	transport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/pkg/logger"
)

var (
	// ErrServiceNotConnected 表示服务未连接
	ErrServiceNotConnected = errors.New("service not connected")
)

// RuntimeStatus 定义运行时状态
type RuntimeStatus struct {
	ServiceID             string               `json:"service_id"`
	Status                entity.ServiceStatus `json:"status"`
	TransportType         string               `json:"transport_type"`
	SessionIDExists       bool                 `json:"session_id_exists"`
	ProtocolVersion       string               `json:"protocol_version,omitempty"`
	ListenEnabled         bool                 `json:"listen_enabled"`
	ListenActive          bool                 `json:"listen_active"`
	ListenLastError       string               `json:"listen_last_error,omitempty"`
	LastSeenAt            *time.Time           `json:"last_seen_at,omitempty"`
	TransportCapabilities map[string]any       `json:"transport_capabilities,omitempty"`
	LastError             string               `json:"last_error,omitempty"`
	FailureCount          int                  `json:"failure_count"`
}

type managedClient struct {
	service         *entity.MCPService
	client          *mcpgoclient.Client
	runtime         RuntimeStatus
	mu              sync.RWMutex
	actualTransport entity.TransportType
	closeOnce       sync.Once
}

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

	return mc, mc.initialize(appCfg)
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

// Manager 定义连接管理器
type Manager struct {
	appCfg config.AppConfig
	mu     sync.RWMutex
	items  map[string]*managedClient
}

// NewManager 创建连接管理器
func NewManager(appCfg config.AppConfig) *Manager {
	return &Manager{appCfg: appCfg, items: make(map[string]*managedClient)}
}

// Connect 建立服务连接
func (m *Manager) Connect(ctx context.Context, service *entity.MCPService) (RuntimeStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.items[service.ID]; ok {
		_ = old.close()
		delete(m.items, service.ID)
	}

	client, err := newManagedClient(m.appCfg, service)
	if err != nil {
		return RuntimeStatus{}, err
	}
	m.items[service.ID] = client
	return client.runtimeStatus(), nil
}

// Disconnect 断开服务连接
func (m *Manager) Disconnect(ctx context.Context, serviceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	client, ok := m.items[serviceID]
	if !ok {
		return ErrServiceNotConnected
	}
	if err := client.close(); err != nil {
		return err
	}
	delete(m.items, serviceID)
	return nil
}

// GetStatus 返回服务运行时状态
func (m *Manager) GetStatus(serviceID string) (RuntimeStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	client, ok := m.items[serviceID]
	if !ok {
		return RuntimeStatus{}, false
	}
	return client.runtimeStatus(), true
}

// ListTools 获取工具列表
func (m *Manager) ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, RuntimeStatus, error) {
	client, err := m.get(serviceID)
	if err != nil {
		return nil, RuntimeStatus{}, err
	}
	res, err := client.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		client.markError(err)
		return nil, client.runtimeStatus(), err
	}
	client.markSeen()
	return res.Tools, client.runtimeStatus(), nil
}

// CallTool 调用工具
func (m *Manager) CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, RuntimeStatus, error) {
	client, err := m.get(serviceID)
	if err != nil {
		return nil, RuntimeStatus{}, err
	}
	res, err := client.client.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}})
	if err != nil {
		client.markError(err)
		return nil, client.runtimeStatus(), err
	}
	client.markSeen()
	return res, client.runtimeStatus(), nil
}

// Ping 执行心跳
func (m *Manager) Ping(ctx context.Context, serviceID string) (RuntimeStatus, error) {
	client, err := m.get(serviceID)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if err := client.client.Ping(ctx); err != nil {
		client.markError(err)
		return client.runtimeStatus(), err
	}
	client.markSeen()
	return client.runtimeStatus(), nil
}

// IDs 返回当前连接 ID
func (m *Manager) IDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.items))
	for id := range m.items {
		ids = append(ids, id)
	}
	return ids
}

// get 获取指定服务对应的受管客户端
func (m *Manager) get(serviceID string) (*managedClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	client, ok := m.items[serviceID]
	if !ok {
		return nil, ErrServiceNotConnected
	}
	return client, nil
}

// applyHealthState 将健康检查结果同步到内存运行态
func (m *Manager) applyHealthState(serviceID string, status entity.ServiceStatus, failureCount int, lastError string) {
	m.mu.RLock()
	client, ok := m.items[serviceID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	client.applyHealthState(status, failureCount, lastError)
}

// markError 在调用失败时记录最新错误状态
func (m *managedClient) markError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtime.LastError = err.Error()
	m.runtime.ListenLastError = err.Error()
	m.runtime.ListenActive = false
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

// HealthChecker 定义健康检查器
type HealthChecker struct {
	manager          *Manager
	interval         time.Duration
	timeout          time.Duration
	failureThreshold int
	updateFn         func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error
	idsFn            func() []string
	pingFn           func(ctx context.Context, serviceID string) (RuntimeStatus, error)
	listToolsFn      func(ctx context.Context, serviceID string) ([]mcp.Tool, RuntimeStatus, error)
	syncRuntimeFn    func(serviceID string, status entity.ServiceStatus, failureCount int, lastError string)
	stop             chan struct{}
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(manager *Manager, cfg config.HealthCheckConfig, updateFn func(ctx context.Context, serviceID string, status entity.ServiceStatus, failureCount int, lastError string) error) *HealthChecker {
	return &HealthChecker{
		manager:          manager,
		interval:         cfg.Interval,
		timeout:          cfg.Timeout,
		failureThreshold: cfg.FailureThreshold,
		updateFn:         updateFn,
		idsFn:            manager.IDs,
		pingFn:           manager.Ping,
		listToolsFn:      manager.ListTools,
		syncRuntimeFn:    manager.applyHealthState,
		stop:             make(chan struct{}),
	}
}

// Start 启动健康检查
func (h *HealthChecker) Start() {
	if h.interval <= 0 {
		h.interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.checkOnce()
			case <-h.stop:
				return
			}
		}
	}()
}

// checkOnce 对全部已连接服务执行一轮健康检查
func (h *HealthChecker) checkOnce() {
	var wg sync.WaitGroup
	for _, serviceID := range h.idsFn() {
		serviceID := serviceID
		wg.Go(func() {

			ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
			defer cancel()

			status, err := h.pingFn(ctx, serviceID)
			if err == nil {
				h.markHealthy(serviceID)
				return
			}

			// 部分服务未实现标准 ping，这里退化为 list_tools 探活，减少误报
			if isUnsupportedPingError(err) && h.listToolsFn != nil {
				pingErr := err
				if _, runtimeStatus, fallbackErr := h.listToolsFn(ctx, serviceID); fallbackErr == nil {
					_ = runtimeStatus
					h.markHealthy(serviceID)
					return
				} else {
					status = runtimeStatus
					h.markFailure(serviceID, status, fmt.Sprintf("unsupported_ping: %s; fallback failed: %s", pingErr.Error(), fallbackErr.Error()))
					return
				}
			}

			h.markFailure(serviceID, status, err)
		})
	}
	wg.Wait()
}

// Stop 停止健康检查
func (h *HealthChecker) Stop() {
	select {
	case <-h.stop:
	default:
		close(h.stop)
	}
}

// markHealthy 将服务状态重置为健康
func (h *HealthChecker) markHealthy(serviceID string) {
	if h.syncRuntimeFn != nil {
		h.syncRuntimeFn(serviceID, entity.ServiceStatusConnected, 0, "")
	}
	if h.updateFn != nil {
		_ = h.updateFn(context.Background(), serviceID, entity.ServiceStatusConnected, 0, "")
	}
}

// markFailure 根据失败次数推进服务状态并落库
func (h *HealthChecker) markFailure(serviceID string, status RuntimeStatus, failure any) {
	next := status.FailureCount + 1
	svcStatus := entity.ServiceStatusConnected
	if next >= h.failureThreshold {
		svcStatus = entity.ServiceStatusError
	}
	lastError := formatHealthFailure(failure)
	if h.syncRuntimeFn != nil {
		h.syncRuntimeFn(serviceID, svcStatus, next, lastError)
	}
	if h.updateFn != nil {
		_ = h.updateFn(context.Background(), serviceID, svcStatus, next, lastError)
	}
}

// formatHealthFailure 将失败原因统一格式化为可读文本
func formatHealthFailure(failure any) string {
	switch v := failure.(type) {
	case nil:
		return ""
	case string:
		return v
	case error:
		return fmt.Sprintf("%s: %s", classifyHealthError(v), v.Error())
	default:
		return fmt.Sprint(v)
	}
}

// classifyHealthError 对健康检查错误进行分类
func classifyHealthError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrServiceNotConnected) {
		return "disconnected"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	return "invoke_failed"
}

// isUnsupportedPingError 判断错误是否表示服务端不支持 ping
func isUnsupportedPingError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "method not found") ||
		strings.Contains(message, "-32601") ||
		strings.Contains(message, "not supported")
}

// MarshalResult 将调用结果转为通用 JSON
func MarshalResult(result *mcp.CallToolResult) map[string]any {
	if result == nil {
		return nil
	}
	payload := map[string]any{
		"is_error": result.IsError,
	}
	if result.StructuredContent != nil {
		payload["structured_content"] = result.StructuredContent
	}
	if len(result.Content) > 0 {
		parts := make([]any, 0, len(result.Content))
		for _, item := range result.Content {
			raw, _ := json.Marshal(item)
			var decoded any
			_ = json.Unmarshal(raw, &decoded)
			parts = append(parts, decoded)
		}
		payload["content"] = parts
	}
	return payload
}
