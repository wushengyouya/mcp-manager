package service

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
)

// ServiceConnector 定义服务连接控制接口。
type ServiceConnector interface {
	Connect(ctx context.Context, service *entity.MCPService) (mcpclient.RuntimeStatus, error)
	Disconnect(ctx context.Context, serviceID string) error
}

// RuntimeStatusReader 定义运行时状态读取接口。
type RuntimeStatusReader interface {
	GetStatus(serviceID string) (mcpclient.RuntimeStatus, bool)
}

// RuntimeStore 定义共享运行态快照存储接口。
type RuntimeStore interface {
	SaveSnapshot(ctx context.Context, snapshot mcpclient.RuntimeSnapshot) error
	GetSnapshot(ctx context.Context, serviceID string) (mcpclient.RuntimeSnapshot, bool, error)
	DeleteSnapshot(ctx context.Context, serviceID string) error
}

// ToolCatalogExecutor 定义工具目录读取接口。
type ToolCatalogExecutor interface {
	ListTools(ctx context.Context, serviceID string) ([]mcp.Tool, mcpclient.RuntimeStatus, error)
}

// ToolInvoker 定义工具调用接口。
type ToolInvoker interface {
	CallTool(ctx context.Context, serviceID, name string, args map[string]any) (*mcp.CallToolResult, mcpclient.RuntimeStatus, error)
}
