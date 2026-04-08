package service

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
)

const defaultRemoteStatusTimeout = 3 * time.Second

// RemoteRuntimeClient 定义 control-plane 访问远程 executor 的最小能力集。
type RemoteRuntimeClient interface {
	Connect(ctx context.Context, service *entity.MCPService) (mcpclient.RuntimeStatus, error)
	Disconnect(ctx context.Context, serviceID string) error
	GetStatus(ctx context.Context, serviceID string) (mcpclient.RuntimeStatus, bool, error)
	ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error)
	CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error)
}

// RemoteRuntimeAdapterOption 定义远程运行时适配器选项。
type RemoteRuntimeAdapterOption func(*RemoteRuntimeAdapter)

// WithRemoteRuntimeSnapshotStore 注入远程运行态快照存储。
func WithRemoteRuntimeSnapshotStore(store RuntimeStore) RemoteRuntimeAdapterOption {
	return func(a *RemoteRuntimeAdapter) {
		if store != nil {
			a.runtimeStore = store
		}
	}
}

// WithRemoteRuntimeStatusTimeout 注入远程状态查询超时。
func WithRemoteRuntimeStatusTimeout(timeout time.Duration) RemoteRuntimeAdapterOption {
	return func(a *RemoteRuntimeAdapter) {
		if timeout > 0 {
			a.statusTimeout = timeout
		}
	}
}

// RemoteRuntimeAdapter 通过远程 executor 满足运行时接口。
type RemoteRuntimeAdapter struct {
	client        RemoteRuntimeClient
	runtimeStore  RuntimeStore
	statusTimeout time.Duration
}

// NewRemoteRuntimeAdapter 创建远程运行时适配器。
func NewRemoteRuntimeAdapter(client RemoteRuntimeClient, opts ...RemoteRuntimeAdapterOption) *RemoteRuntimeAdapter {
	adapter := &RemoteRuntimeAdapter{
		client:        client,
		runtimeStore:  NoopRuntimeStore{},
		statusTimeout: defaultRemoteStatusTimeout,
	}
	for _, opt := range opts {
		opt(adapter)
	}
	return adapter
}

// Connect 建立远程服务连接。
func (a *RemoteRuntimeAdapter) Connect(ctx context.Context, service *entity.MCPService) (mcpclient.RuntimeStatus, error) {
	status, err := a.client.Connect(ctx, service)
	if err == nil {
		a.persistSnapshot(ctx, status)
	}
	return status, err
}

// Disconnect 断开远程服务连接。
func (a *RemoteRuntimeAdapter) Disconnect(ctx context.Context, serviceID string) error {
	if err := a.client.Disconnect(ctx, serviceID); err != nil {
		return err
	}
	if err := a.runtimeStore.DeleteSnapshot(ctx, serviceID); err != nil {
		logRuntimeSnapshotError("delete", serviceID, err)
	}
	return nil
}

// GetStatus 读取远程服务运行时状态。
func (a *RemoteRuntimeAdapter) GetStatus(serviceID string) (mcpclient.RuntimeStatus, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), a.statusTimeout)
	defer cancel()

	status, ok, err := a.client.GetStatus(ctx, serviceID)
	if err != nil {
		logRuntimeSnapshotError("get_status", serviceID, err)
		return mcpclient.RuntimeStatus{}, false
	}
	if ok {
		a.persistSnapshot(ctx, status)
	}
	return status, ok
}

// ListTools 获取远程工具目录。
func (a *RemoteRuntimeAdapter) ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	tools, status, err := a.client.ListTools(ctx, serviceID)
	if status.ServiceID != "" {
		a.persistSnapshot(ctx, status)
	}
	return tools, status, err
}

// CallTool 调用远程工具。
func (a *RemoteRuntimeAdapter) CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	result, status, err := a.client.CallTool(ctx, serviceID, name, args)
	if status.ServiceID != "" {
		a.persistSnapshot(ctx, status)
	}
	return result, status, err
}

func (a *RemoteRuntimeAdapter) persistSnapshot(ctx context.Context, status mcpclient.RuntimeStatus) {
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
