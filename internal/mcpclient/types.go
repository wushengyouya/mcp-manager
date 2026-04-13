package mcpclient

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
)

var (
	// ErrServiceNotConnected 表示服务未连接
	ErrServiceNotConnected = errors.New("service not connected")
	// ErrSessionRequired 表示服务要求建立 MCP 会话但未成功建立
	ErrSessionRequired = errors.New("streamable_http session required but not established")
	// ErrSessionDisabled 表示服务配置为禁用会话但服务端返回了会话
	ErrSessionDisabled = errors.New("streamable_http session disabled but server returned session")
	// ErrSessionReinitializeRequired 表示会话已失效，必须重新连接
	ErrSessionReinitializeRequired = errors.New("streamable_http session terminated, reconnect required")
)

// RuntimeStatus 定义运行时状态
type RuntimeStatus struct {
	ServiceID             string               `json:"service_id"`
	Status                entity.ServiceStatus `json:"status"`
	TransportType         string               `json:"transport_type"`
	SessionIDExists       bool                 `json:"session_id_exists"`
	ProtocolVersion       string               `json:"protocol_version,omitempty"`
	ExecutorID            string               `json:"executor_id,omitempty"`
	SnapshotWriter        string               `json:"snapshot_writer,omitempty"`
	RequestID             string               `json:"request_id,omitempty"`
	ListenEnabled         bool                 `json:"listen_enabled"`
	ListenActive          bool                 `json:"listen_active"`
	ListenLastError       string               `json:"listen_last_error,omitempty"`
	ConnectedAt           *time.Time           `json:"connected_at,omitempty"`
	LastSeenAt            *time.Time           `json:"last_seen_at,omitempty"`
	LastUsedAt            *time.Time           `json:"last_used_at,omitempty"`
	InFlight              int                  `json:"in_flight"`
	TransportCapabilities map[string]any       `json:"transport_capabilities,omitempty"`
	LastError             string               `json:"last_error,omitempty"`
	FailureCount          int                  `json:"failure_count"`
}

// RuntimeSnapshot 定义共享运行态快照。
type RuntimeSnapshot struct {
	RuntimeStatus
	ObservedAt time.Time `json:"observed_at"`
}

type runtimeClient interface {
	Start(ctx context.Context) error
	Initialize(ctx context.Context, request mcp.InitializeRequest) (*mcp.InitializeResult, error)
	Close() error
	ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
	Ping(ctx context.Context) error
	GetSessionId() string
	OnNotification(handler func(notification mcp.JSONRPCNotification))
	OnConnectionLost(handler func(error))
}

// managedClient 保存服务配置、底层连接和运行态快照。
type managedClient struct {
	service         *entity.MCPService
	client          runtimeClient
	runtime         RuntimeStatus
	mu              sync.RWMutex
	actualTransport entity.TransportType
	closeOnce       sync.Once
}

// IdleDurationAt 返回最近业务使用时间距当前的空闲时长。
func (s RuntimeStatus) IdleDurationAt(now time.Time) (time.Duration, bool) {
	if s.LastUsedAt == nil || s.LastUsedAt.IsZero() {
		return 0, false
	}
	return nonNegativeDuration(now.Sub(*s.LastUsedAt)), true
}

// ConnectedDurationAt 返回当前连接已维持的时长。
func (s RuntimeStatus) ConnectedDurationAt(now time.Time) (time.Duration, bool) {
	if s.ConnectedAt == nil || s.ConnectedAt.IsZero() {
		return 0, false
	}
	return nonNegativeDuration(now.Sub(*s.ConnectedAt)), true
}

// WouldReapAt 返回按 idle_timeout 诊断时当前连接是否会命中回收条件。
func (s RuntimeStatus) WouldReapAt(now time.Time, idleTimeout time.Duration) bool {
	if idleTimeout <= 0 {
		return false
	}
	if s.Status != entity.ServiceStatusConnected {
		return false
	}
	if s.ListenEnabled || s.InFlight > 0 {
		return false
	}
	idleDuration, ok := s.IdleDurationAt(now)
	return ok && idleDuration >= idleTimeout
}

func nonNegativeDuration(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	return d
}
