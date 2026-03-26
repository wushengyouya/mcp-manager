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
	ListenEnabled         bool                 `json:"listen_enabled"`
	ListenActive          bool                 `json:"listen_active"`
	ListenLastError       string               `json:"listen_last_error,omitempty"`
	LastSeenAt            *time.Time           `json:"last_seen_at,omitempty"`
	TransportCapabilities map[string]any       `json:"transport_capabilities,omitempty"`
	LastError             string               `json:"last_error,omitempty"`
	FailureCount          int                  `json:"failure_count"`
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

type managedClient struct {
	service         *entity.MCPService
	client          runtimeClient
	runtime         RuntimeStatus
	mu              sync.RWMutex
	actualTransport entity.TransportType
	closeOnce       sync.Once
}
