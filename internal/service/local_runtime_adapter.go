package service

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
)

// LocalRuntimeAdapter 使用本地 manager 满足运行时接口。
type LocalRuntimeAdapter struct {
	manager *mcpclient.Manager
}

// NewLocalRuntimeAdapter 创建本地运行时适配器。
func NewLocalRuntimeAdapter(manager *mcpclient.Manager) *LocalRuntimeAdapter {
	return &LocalRuntimeAdapter{manager: manager}
}

// Connect 建立服务连接。
func (a *LocalRuntimeAdapter) Connect(ctx context.Context, service *entity.MCPService) (mcpclient.RuntimeStatus, error) {
	return a.manager.Connect(ctx, service)
}

// Disconnect 断开服务连接。
func (a *LocalRuntimeAdapter) Disconnect(ctx context.Context, serviceID string) error {
	return a.manager.Disconnect(ctx, serviceID)
}

// GetStatus 读取服务运行时状态。
func (a *LocalRuntimeAdapter) GetStatus(serviceID string) (mcpclient.RuntimeStatus, bool) {
	return a.manager.GetStatus(serviceID)
}

// ListTools 获取工具目录。
func (a *LocalRuntimeAdapter) ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	return a.manager.ListTools(ctx, serviceID)
}

// CallTool 调用工具。
func (a *LocalRuntimeAdapter) CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	return a.manager.CallTool(ctx, serviceID, name, args)
}
