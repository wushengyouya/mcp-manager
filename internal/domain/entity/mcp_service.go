package entity

// TransportType 定义传输类型。
type TransportType string

// ServiceStatus 定义服务状态。
type ServiceStatus string

const (
	// TransportTypeStdio 表示本地 stdio。
	TransportTypeStdio TransportType = "stdio"
	// TransportTypeStreamableHTTP 表示 streamable_http。
	TransportTypeStreamableHTTP TransportType = "streamable_http"
	// TransportTypeSSE 表示 sse。
	TransportTypeSSE TransportType = "sse"

	// ServiceStatusDisconnected 表示未连接。
	ServiceStatusDisconnected ServiceStatus = "DISCONNECTED"
	// ServiceStatusConnecting 表示连接中。
	ServiceStatusConnecting ServiceStatus = "CONNECTING"
	// ServiceStatusConnected 表示已连接。
	ServiceStatusConnected ServiceStatus = "CONNECTED"
	// ServiceStatusError 表示错误。
	ServiceStatusError ServiceStatus = "ERROR"
)

// MCPService 定义 MCP 服务配置实体。
type MCPService struct {
	Base
	Name          string         `gorm:"type:varchar(100);not null;index:idx_mcp_services_name_active,unique,where:deleted_at IS NULL" json:"name"`
	Description   string         `gorm:"type:text" json:"description"`
	TransportType TransportType  `gorm:"type:varchar(20);not null" json:"transport_type"`
	Command       string         `gorm:"type:varchar(500)" json:"command"`
	Args          JSONStringList `gorm:"type:json" json:"args"`
	Env           JSONStringMap  `gorm:"type:json" json:"env"`
	URL           string         `gorm:"type:varchar(500)" json:"url"`
	BearerToken   string         `gorm:"type:text" json:"bearer_token,omitempty"`
	CustomHeaders JSONStringMap  `gorm:"type:json" json:"custom_headers"`
	SessionMode   string         `gorm:"type:varchar(20);default:auto" json:"session_mode"`
	CompatMode    string         `gorm:"type:varchar(30);default:off" json:"compat_mode"`
	ListenEnabled bool           `gorm:"default:false" json:"listen_enabled"`
	Timeout       int            `gorm:"default:30" json:"timeout"`
	Status        ServiceStatus  `gorm:"type:varchar(20);default:DISCONNECTED" json:"status"`
	FailureCount  int            `gorm:"default:0" json:"failure_count"`
	LastError     string         `gorm:"type:text" json:"last_error"`
	Tags          JSONStringList `gorm:"type:json" json:"tags"`
}

// IsRemote 判断是否为远程服务。
func (s MCPService) IsRemote() bool {
	return s.TransportType == TransportTypeStreamableHTTP || s.TransportType == TransportTypeSSE
}
