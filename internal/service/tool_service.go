package service

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// ToolService 定义工具元数据服务。
type ToolService interface {
	Sync(ctx context.Context, serviceID string, actor AuditEntry) ([]entity.Tool, error)
	ListByService(ctx context.Context, serviceID string) ([]entity.Tool, error)
	Get(ctx context.Context, toolID string) (*entity.Tool, error)
}

type toolService struct {
	tools    repository.ToolRepository
	services repository.MCPServiceRepository
	manager  *mcpclient.Manager
	audit    AuditSink
}

// NewToolService 创建工具服务。
func NewToolService(tools repository.ToolRepository, services repository.MCPServiceRepository, manager *mcpclient.Manager, audit AuditSink) ToolService {
	if audit == nil {
		audit = NoopAuditSink{}
	}
	return &toolService{tools: tools, services: services, manager: manager, audit: audit}
}

func (s *toolService) Sync(ctx context.Context, serviceID string, actor AuditEntry) ([]entity.Tool, error) {
	service, err := s.services.GetByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if service.Status == entity.ServiceStatusError {
		return nil, response.NewBizError(http.StatusConflict, response.CodeConflict, "服务处于错误状态，请先恢复连接", nil)
	}
	items, runtimeStatus, err := s.manager.ListTools(ctx, serviceID)
	if err != nil {
		return nil, response.NewBizError(http.StatusBadGateway, response.CodeToolInvokeFailed, "同步工具失败", err)
	}
	tools := make([]entity.Tool, 0, len(items))
	now := time.Now()
	for _, item := range items {
		schema := entity.JSONMap{}
		if raw, err := json.Marshal(item.InputSchema); err == nil {
			_ = json.Unmarshal(raw, &schema)
		}
		tools = append(tools, entity.Tool{
			MCPServiceID: serviceID,
			Name:         item.Name,
			Description:  item.Description,
			InputSchema:  schema,
			IsEnabled:    true,
			SyncedAt:     now,
		})
	}
	if err := s.tools.BatchUpsert(ctx, tools); err != nil {
		return nil, err
	}
	actor.Action = "sync_tools"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = serviceID
	actor.Detail = map[string]any{
		"tool_count":       len(tools),
		"transport_type":   runtimeStatus.TransportType,
		"protocol_version": runtimeStatus.ProtocolVersion,
		"service_name":     service.Name,
	}
	_ = s.audit.Record(ctx, actor)
	return s.tools.ListByService(ctx, serviceID)
}

func (s *toolService) ListByService(ctx context.Context, serviceID string) ([]entity.Tool, error) {
	return s.tools.ListByService(ctx, serviceID)
}

func (s *toolService) Get(ctx context.Context, toolID string) (*entity.Tool, error) {
	return s.tools.GetByID(ctx, toolID)
}
