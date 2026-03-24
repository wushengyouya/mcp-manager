package mcpclient

import (
	"errors"
	"sync"
	"time"

	mcpgoclient "github.com/mark3labs/mcp-go/client"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
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
