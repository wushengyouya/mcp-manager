package service

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/pkg/logger"
)

const defaultRemoteStatusTimeout = 3 * time.Second
const defaultOwnerConflictDetectionWindow = 30 * time.Second

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

// OwnerConflictEvent 定义 owner 冲突检测事件。
type OwnerConflictEvent struct {
	OwnerConflictDetected bool
	ServiceID             string
	OldExecutorID         string
	NewExecutorID         string
	Operation             string
	RequestID             string
	WindowMS              int64
}

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

// WithRemoteOwnerConflictDetection 注入 owner 冲突检测配置。
func WithRemoteOwnerConflictDetection(cfg config.RuntimeConfig) RemoteRuntimeAdapterOption {
	return func(a *RemoteRuntimeAdapter) {
		a.ownerConflictDetectionEnabled = cfg.OwnerConflictDetectionEnabled
	}
}

// WithRemoteOwnerConflictReporter 注入 owner 冲突事件回调。
func WithRemoteOwnerConflictReporter(reporter func(OwnerConflictEvent)) RemoteRuntimeAdapterOption {
	return func(a *RemoteRuntimeAdapter) {
		if reporter != nil {
			a.ownerConflictReporter = reporter
		}
	}
}

// WithRemoteOwnerConflictWindow 注入 owner 冲突检测窗口。
func WithRemoteOwnerConflictWindow(window time.Duration) RemoteRuntimeAdapterOption {
	return func(a *RemoteRuntimeAdapter) {
		if window > 0 {
			a.ownerConflictWindow = window
		}
	}
}

// RemoteRuntimeAdapter 通过远程 executor 满足运行时接口。
type RemoteRuntimeAdapter struct {
	client                        RemoteRuntimeClient
	runtimeStore                  RuntimeStore
	statusTimeout                 time.Duration
	ownerConflictDetectionEnabled bool
	ownerConflictWindow           time.Duration
	ownerConflictReporter         func(OwnerConflictEvent)
	nowFn                         func() time.Time
}

// NewRemoteRuntimeAdapter 创建远程运行时适配器。
func NewRemoteRuntimeAdapter(client RemoteRuntimeClient, opts ...RemoteRuntimeAdapterOption) *RemoteRuntimeAdapter {
	adapter := &RemoteRuntimeAdapter{
		client:                client,
		runtimeStore:          NoopRuntimeStore{},
		statusTimeout:         defaultRemoteStatusTimeout,
		ownerConflictWindow:   defaultOwnerConflictDetectionWindow,
		ownerConflictReporter: logOwnerConflictEvent,
		nowFn:                 time.Now,
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
		a.persistSnapshot(ctx, "connect", status)
	}
	a.logOwnerEvidence("connect", status)
	return status, err
}

// Disconnect 断开远程服务连接。
func (a *RemoteRuntimeAdapter) Disconnect(ctx context.Context, serviceID string) error {
	status := mcpclient.RuntimeStatus{ServiceID: serviceID}
	if clientWithEvidence, ok := a.client.(interface {
		DisconnectWithEvidence(ctx context.Context, serviceID string) (mcpclient.RuntimeStatus, error)
	}); ok {
		detail, err := clientWithEvidence.DisconnectWithEvidence(ctx, serviceID)
		status = detail
		if err != nil {
			return err
		}
		a.handlePotentialOwnerConflict(ctx, "disconnect", status)
		a.logOwnerEvidence("disconnect", status)
	} else {
		if err := a.client.Disconnect(ctx, serviceID); err != nil {
			return err
		}
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
		a.persistSnapshot(ctx, "status", status)
		a.logOwnerEvidence("status", status)
	}
	return status, ok
}

// ListTools 获取远程工具目录。
func (a *RemoteRuntimeAdapter) ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error) {
	tools, status, err := a.client.ListTools(ctx, serviceID)
	if status.ServiceID != "" {
		a.persistSnapshot(ctx, "list_tools", status)
	}
	a.logOwnerEvidence("list_tools", status)
	return tools, status, err
}

// CallTool 调用远程工具。
func (a *RemoteRuntimeAdapter) CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error) {
	result, status, err := a.client.CallTool(ctx, serviceID, name, args)
	if status.ServiceID != "" {
		a.persistSnapshot(ctx, "invoke", status)
	}
	a.logOwnerEvidence("invoke", status)
	return result, status, err
}

func (a *RemoteRuntimeAdapter) persistSnapshot(ctx context.Context, operation string, status mcpclient.RuntimeStatus) {
	if a.runtimeStore == nil {
		return
	}
	if status.SnapshotWriter == "" {
		status.SnapshotWriter = status.ExecutorID
	}
	a.handlePotentialOwnerConflict(ctx, operation, status)
	snapshot := mcpclient.RuntimeSnapshot{
		RuntimeStatus: status,
		ObservedAt:    a.nowFn(),
	}
	if err := a.runtimeStore.SaveSnapshot(ctx, snapshot); err != nil {
		logRuntimeSnapshotError("save", status.ServiceID, err)
	}
}

func (a *RemoteRuntimeAdapter) logOwnerEvidence(operation string, status mcpclient.RuntimeStatus) {
	if status.ServiceID == "" {
		return
	}
	logger.S().Infow(
		"owner 诊断快照",
		"executor_id", status.ExecutorID,
		"request_id", status.RequestID,
		"service_id", status.ServiceID,
		"operation", operation,
		"snapshot_writer", status.SnapshotWriter,
	)
}

func (a *RemoteRuntimeAdapter) handlePotentialOwnerConflict(ctx context.Context, operation string, status mcpclient.RuntimeStatus) {
	if !a.ownerConflictDetectionEnabled || a.runtimeStore == nil || status.ServiceID == "" || status.ExecutorID == "" {
		return
	}
	previous, ok, err := a.runtimeStore.GetSnapshot(ctx, status.ServiceID)
	if err != nil {
		logRuntimeSnapshotError("owner_conflict_lookup", status.ServiceID, err)
		return
	}
	if !ok || previous.ExecutorID == "" || previous.ExecutorID == status.ExecutorID {
		return
	}
	if a.nowFn().Sub(previous.ObservedAt) > a.ownerConflictWindow {
		return
	}
	if a.ownerConflictReporter != nil {
		a.ownerConflictReporter(OwnerConflictEvent{
			OwnerConflictDetected: true,
			ServiceID:             status.ServiceID,
			OldExecutorID:         previous.ExecutorID,
			NewExecutorID:         status.ExecutorID,
			Operation:             operation,
			RequestID:             status.RequestID,
			WindowMS:              a.ownerConflictWindow.Milliseconds(),
		})
	}
}

func logOwnerConflictEvent(event OwnerConflictEvent) {
	logger.S().Warnw(
		"owner 冲突检测",
		"owner_conflict_detected", event.OwnerConflictDetected,
		"service_id", event.ServiceID,
		"old_executor_id", event.OldExecutorID,
		"new_executor_id", event.NewExecutorID,
		"operation", event.Operation,
		"request_id", event.RequestID,
		"window_ms", event.WindowMS,
	)
}
