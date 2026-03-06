package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// CreateMCPServiceInput 定义创建服务输入。
type CreateMCPServiceInput struct {
	Name          string
	Description   string
	TransportType entity.TransportType
	Command       string
	Args          []string
	Env           map[string]string
	URL           string
	BearerToken   string
	CustomHeaders map[string]string
	SessionMode   string
	CompatMode    string
	ListenEnabled bool
	Timeout       int
	Tags          []string
}

// MCPService 定义服务业务接口。
type MCPService interface {
	Create(ctx context.Context, input CreateMCPServiceInput, actor AuditEntry) (*entity.MCPService, error)
	Update(ctx context.Context, id string, input CreateMCPServiceInput, actor AuditEntry) (*entity.MCPService, error)
	Delete(ctx context.Context, id string, actor AuditEntry) error
	Get(ctx context.Context, id string) (*entity.MCPService, error)
	List(ctx context.Context, filter repository.MCPServiceListFilter) ([]entity.MCPService, int64, error)
	Connect(ctx context.Context, id string, actor AuditEntry) (mcpclient.RuntimeStatus, error)
	Disconnect(ctx context.Context, id string, actor AuditEntry) error
	Status(ctx context.Context, id string) (map[string]any, error)
}

type mcpService struct {
	repo    repository.MCPServiceRepository
	manager *mcpclient.Manager
	audit   AuditSink
}

// NewMCPService 创建服务业务实现。
func NewMCPService(repo repository.MCPServiceRepository, manager *mcpclient.Manager, audit AuditSink) MCPService {
	if audit == nil {
		audit = NoopAuditSink{}
	}
	return &mcpService{repo: repo, manager: manager, audit: audit}
}

func (s *mcpService) Create(ctx context.Context, input CreateMCPServiceInput, actor AuditEntry) (*entity.MCPService, error) {
	service, err := buildServiceEntity(input)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, service); err != nil {
		return nil, err
	}
	actor.Action = "create_service"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = service.ID
	actor.Detail = sanitizedServiceDetail(service)
	_ = s.audit.Record(ctx, actor)
	return service, nil
}

func (s *mcpService) Update(ctx context.Context, id string, input CreateMCPServiceInput, actor AuditEntry) (*entity.MCPService, error) {
	service, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	next, err := buildServiceEntity(input)
	if err != nil {
		return nil, err
	}
	next.Base = service.Base
	if next.BearerToken == "" {
		next.BearerToken = service.BearerToken
	}
	if err := s.repo.Update(ctx, next); err != nil {
		return nil, err
	}
	actor.Action = "update_service"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = next.ID
	actor.Detail = sanitizedServiceDetail(next)
	_ = s.audit.Record(ctx, actor)
	return next, nil
}

func (s *mcpService) Delete(ctx context.Context, id string, actor AuditEntry) error {
	if _, ok := s.manager.GetStatus(id); ok {
		return response.NewBizError(http.StatusBadRequest, response.CodeInvalidArgument, "请先断开服务连接", nil)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	actor.Action = "delete_service"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = id
	_ = s.audit.Record(ctx, actor)
	return nil
}

func (s *mcpService) Get(ctx context.Context, id string) (*entity.MCPService, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *mcpService) List(ctx context.Context, filter repository.MCPServiceListFilter) ([]entity.MCPService, int64, error) {
	return s.repo.List(ctx, filter)
}

func (s *mcpService) Connect(ctx context.Context, id string, actor AuditEntry) (mcpclient.RuntimeStatus, error) {
	service, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return mcpclient.RuntimeStatus{}, err
	}
	_ = s.repo.UpdateStatus(ctx, id, entity.ServiceStatusConnecting, service.FailureCount, "")
	status, err := s.manager.Connect(ctx, service)
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, id, entity.ServiceStatusError, service.FailureCount+1, err.Error())
		return mcpclient.RuntimeStatus{}, response.NewBizError(http.StatusBadGateway, response.CodeServiceConnectFailed, "服务连接失败", err)
	}
	_ = s.repo.UpdateStatus(ctx, id, entity.ServiceStatusConnected, 0, "")
	actor.Action = "connect_service"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = id
	actor.Detail = map[string]any{"transport_type": status.TransportType}
	_ = s.audit.Record(ctx, actor)
	return status, nil
}

func (s *mcpService) Disconnect(ctx context.Context, id string, actor AuditEntry) error {
	if err := s.manager.Disconnect(ctx, id); err != nil && err != mcpclient.ErrServiceNotConnected {
		return err
	}
	if err := s.repo.UpdateStatus(ctx, id, entity.ServiceStatusDisconnected, 0, ""); err != nil {
		return err
	}
	actor.Action = "disconnect_service"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = id
	_ = s.audit.Record(ctx, actor)
	return nil
}

func (s *mcpService) Status(ctx context.Context, id string) (map[string]any, error) {
	service, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"id":             service.ID,
		"name":           service.Name,
		"status":         service.Status,
		"failure_count":  service.FailureCount,
		"last_error":     service.LastError,
		"transport_type": service.TransportType,
	}
	if runtimeStatus, ok := s.manager.GetStatus(id); ok {
		out["status"] = runtimeStatus.Status
		out["session_id_exists"] = runtimeStatus.SessionIDExists
		out["protocol_version"] = runtimeStatus.ProtocolVersion
		out["listen_enabled"] = runtimeStatus.ListenEnabled
		out["listen_active"] = runtimeStatus.ListenActive
		out["listen_last_error"] = runtimeStatus.ListenLastError
		out["last_seen_at"] = runtimeStatus.LastSeenAt
		out["transport_capabilities"] = runtimeStatus.TransportCapabilities
		out["transport_type"] = runtimeStatus.TransportType
		out["last_error"] = runtimeStatus.LastError
	}
	return out, nil
}

func buildServiceEntity(input CreateMCPServiceInput) (*entity.MCPService, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, response.NewBizError(http.StatusBadRequest, response.CodeInvalidArgument, "服务名称不能为空", nil)
	}
	if input.Timeout <= 0 {
		input.Timeout = 30
	}
	switch input.TransportType {
	case entity.TransportTypeStdio:
		if input.Command == "" {
			return nil, response.NewBizError(http.StatusBadRequest, response.CodeInvalidArgument, "stdio 服务必须提供 command", nil)
		}
	case entity.TransportTypeStreamableHTTP, entity.TransportTypeSSE:
		if input.URL == "" {
			return nil, response.NewBizError(http.StatusBadRequest, response.CodeInvalidArgument, "远程服务必须提供 url", nil)
		}
	default:
		return nil, response.NewBizError(http.StatusBadRequest, response.CodeInvalidArgument, "不支持的 transport_type", nil)
	}
	if input.SessionMode == "" {
		input.SessionMode = "auto"
	}
	if input.CompatMode == "" {
		input.CompatMode = "off"
	}
	return &entity.MCPService{
		Name:          input.Name,
		Description:   input.Description,
		TransportType: input.TransportType,
		Command:       input.Command,
		Args:          entity.JSONStringList(input.Args),
		Env:           entity.JSONStringMap(input.Env),
		URL:           input.URL,
		BearerToken:   input.BearerToken,
		CustomHeaders: entity.JSONStringMap(input.CustomHeaders),
		SessionMode:   input.SessionMode,
		CompatMode:    input.CompatMode,
		ListenEnabled: input.ListenEnabled,
		Timeout:       input.Timeout,
		Status:        entity.ServiceStatusDisconnected,
		Tags:          entity.JSONStringList(input.Tags),
	}, nil
}

func sanitizedServiceDetail(service *entity.MCPService) map[string]any {
	headers := map[string]string{}
	for k, v := range service.CustomHeaders {
		lk := strings.ToLower(k)
		if lk == "authorization" || strings.Contains(lk, "token") || strings.Contains(lk, "secret") {
			headers[k] = "***"
		} else {
			headers[k] = v
		}
	}
	detail := map[string]any{
		"name":           service.Name,
		"transport_type": service.TransportType,
		"url":            service.URL,
		"command":        service.Command,
		"custom_headers": headers,
		"session_mode":   service.SessionMode,
		"compat_mode":    service.CompatMode,
		"listen_enabled": service.ListenEnabled,
	}
	if service.BearerToken != "" {
		detail["bearer_token"] = "***"
	}
	return detail
}
