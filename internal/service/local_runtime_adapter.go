package service

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
)

// LocalRuntimeAdapter 使用本地 manager 满足运行时接口。
type LocalRuntimeAdapter struct {
	manager      *mcpclient.Manager
	runtimeStore RuntimeStore
}

// NewLocalRuntimeAdapter 创建本地运行时适配器。
func NewLocalRuntimeAdapter(manager *mcpclient.Manager, stores ...RuntimeStore) *LocalRuntimeAdapter {
	var store RuntimeStore = NoopRuntimeStore{}
	if len(stores) > 0 && stores[0] != nil {
		store = stores[0]
	}
	return &LocalRuntimeAdapter{manager: manager, runtimeStore: store}
}

// Connect 建立服务连接。
func (a *LocalRuntimeAdapter) Connect(ctx context.Context, service *entity.MCPService) (mcpclient.RuntimeStatus, error) {
	status, err := a.manager.Connect(ctx, service)
	if err == nil {
		a.persistSnapshot(ctx, status)
	}
	return status, err
}

// Disconnect 断开服务连接。
func (a *LocalRuntimeAdapter) Disconnect(ctx context.Context, serviceID string) error {
	if err := a.manager.Disconnect(ctx, serviceID); err != nil {
		return err
	}
	if err := a.runtimeStore.DeleteSnapshot(ctx, serviceID); err != nil {
		logRuntimeSnapshotError("delete", serviceID, err)
	}
	return nil
}

// GetStatus 读取服务运行时状态。
func (a *LocalRuntimeAdapter) GetStatus(serviceID string) (mcpclient.RuntimeStatus, bool) {
	status, ok := a.manager.GetStatus(serviceID)
	if ok {
		a.persistSnapshot(context.Background(), status)
	}
	return status, ok
}

// ListTools 获取工具目录。
func (a *LocalRuntimeAdapter) ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	tools, status, err := a.manager.ListTools(ctx, serviceID)
	if status.ServiceID != "" {
		a.persistSnapshot(ctx, status)
	}
	return tools, status, err
}

// CallTool 调用工具。
func (a *LocalRuntimeAdapter) CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	result, status, err := a.manager.CallTool(ctx, serviceID, name, args)
	if status.ServiceID != "" {
		a.persistSnapshot(ctx, status)
	}
	return result, status, err
}

func (a *LocalRuntimeAdapter) persistSnapshot(ctx context.Context, status mcpclient.RuntimeStatus) {
	if a.runtimeStore == nil {
		return
	}
	snapshot := mcpclient.RuntimeSnapshot{
		RuntimeStatus: status,
		ObservedAt:    time.Now(),
	}
	if err := a.runtimeStore.SaveSnapshot(ctx, snapshot); err != nil {
		logRuntimeSnapshotError("save", status.ServiceID, err)
	}
}
