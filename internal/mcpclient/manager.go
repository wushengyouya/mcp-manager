package mcpclient

import (
	"context"
	"errors"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
)

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
		status, handledErr := m.handleClientError(serviceID, client, err)
		return nil, status, handledErr
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
		status, handledErr := m.handleClientError(serviceID, client, err)
		return nil, status, handledErr
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
		status, handledErr := m.handleClientError(serviceID, client, err)
		return status, handledErr
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

// handleClientError 统一处理客户端错误并同步会话失效状态。
func (m *Manager) handleClientError(serviceID string, client *managedClient, err error) (RuntimeStatus, error) {
	if !IsSessionReconnectRequired(err) {
		client.markError(err)
		return client.runtimeStatus(), err
	}

	wrapped := wrapSessionReconnectRequired(err)
	status := client.markTerminalError(wrapped)

	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.items[serviceID]; ok && errors.Is(wrapped, ErrSessionReinitializeRequired) && current == client {
		_ = current.close()
		delete(m.items, serviceID)
	}

	return status, wrapped
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
