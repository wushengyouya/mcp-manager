package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/logger"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// CreateMCPServiceInput 定义创建服务输入
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

// MCPService 定义服务业务接口
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

// mcpService 实现 MCP 服务业务接口。
type mcpService struct {
	repo         repository.MCPServiceRepository
	tools        repository.ToolRepository
	connector    ServiceConnector
	statusReader RuntimeStatusReader
	runtimeStore RuntimeStore
	runtimeCfg   config.RuntimeConfig
	audit        AuditSink
	alerts       AlertService
}

// MCPServiceOption 定义服务可选项。
type MCPServiceOption func(*mcpService)

// WithRuntimeSnapshotStore 注入运行态快照存储。
func WithRuntimeSnapshotStore(store RuntimeStore) MCPServiceOption {
	return func(s *mcpService) {
		if store != nil {
			s.runtimeStore = store
		}
	}
}

// WithRuntimeConfig 注入运行态配置。
func WithRuntimeConfig(cfg config.RuntimeConfig) MCPServiceOption {
	return func(s *mcpService) {
		s.runtimeCfg = cfg
	}
}

// NewMCPService 创建服务业务实现
func NewMCPService(repo repository.MCPServiceRepository, tools repository.ToolRepository, connector ServiceConnector, statusReader RuntimeStatusReader, audit AuditSink, alerts AlertService, opts ...MCPServiceOption) MCPService {
	if audit == nil {
		audit = NoopAuditSink{}
	}
	if alerts == nil {
		alerts = noopAlertService{}
	}
	svc := &mcpService{
		repo:         repo,
		tools:        tools,
		connector:    connector,
		statusReader: statusReader,
		runtimeStore: NoopRuntimeStore{},
		audit:        audit,
		alerts:       alerts,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// Create 创建服务配置并记录审计日志
func (s *mcpService) Create(ctx context.Context, input CreateMCPServiceInput, actor AuditEntry) (*entity.MCPService, error) {
	service, err := buildServiceEntity(input)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, service); err != nil {
		return nil, normalizeServiceRepoErr(err)
	}
	actor.Action = "create_service"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = service.ID
	actor.Detail = sanitizedServiceDetail(service)
	_ = s.audit.Record(ctx, actor)
	return service, nil
}

// Update 更新服务配置并保留必要的敏感字段
func (s *mcpService) Update(ctx context.Context, id string, input CreateMCPServiceInput, actor AuditEntry) (*entity.MCPService, error) {
	service, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// 基于最新入参重建服务实体，保证默认值和校验逻辑一致
	next, err := buildServiceEntity(input)
	if err != nil {
		return nil, err
	}
	next.Base = service.Base
	if next.BearerToken == "" {
		next.BearerToken = service.BearerToken
	}
	if err := s.repo.Update(ctx, next); err != nil {
		return nil, normalizeServiceRepoErr(err)
	}
	actor.Action = "update_service"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = next.ID
	actor.Detail = sanitizedServiceDetail(next)
	_ = s.audit.Record(ctx, actor)
	return next, nil
}

// Delete 删除服务，并同步清理连接与工具元数据
func (s *mcpService) Delete(ctx context.Context, id string, actor AuditEntry) error {
	service, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	autoDisconnected := false
	// 删除前先尝试断开运行中的连接，避免内存里遗留失效连接
	if err := s.connector.Disconnect(ctx, id); err == nil {
		autoDisconnected = true
	} else if err != mcpclient.ErrServiceNotConnected {
		return err
	}
	toolDeletedCount := int64(0)
	if s.tools != nil {
		var err error
		toolDeletedCount, err = s.tools.DeleteByService(ctx, id)
		if err != nil {
			return err
		}
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	actor.Action = "delete_service"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = id
	actor.Detail = map[string]any{
		"service_name":            service.Name,
		"transport_type":          service.TransportType,
		"service_endpoint":        serviceEndpoint(service),
		"previous_status":         service.Status,
		"auto_disconnected":       autoDisconnected,
		"tool_soft_deleted_count": toolDeletedCount,
	}
	_ = s.audit.Record(ctx, actor)
	return nil
}

// Get 查询单个服务详情
func (s *mcpService) Get(ctx context.Context, id string) (*entity.MCPService, error) {
	return s.repo.GetByID(ctx, id)
}

// List 分页查询服务列表
func (s *mcpService) List(ctx context.Context, filter repository.MCPServiceListFilter) ([]entity.MCPService, int64, error) {
	return s.repo.List(ctx, filter)
}

// Connect 建立服务连接并同步运行状态
func (s *mcpService) Connect(ctx context.Context, id string, actor AuditEntry) (mcpclient.RuntimeStatus, error) {
	service, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return mcpclient.RuntimeStatus{}, err
	}
	_ = s.repo.UpdateStatus(ctx, id, entity.ServiceStatusConnecting, service.FailureCount, "")
	status, err := s.connector.Connect(ctx, service)
	if err != nil {
		next := service.FailureCount + 1
		_ = s.repo.UpdateStatus(ctx, id, entity.ServiceStatusError, next, err.Error())
		s.recordServiceError(ctx, service, actor, next, err.Error(), service.Status != entity.ServiceStatusError, "connect")
		switch {
		case mcpclient.IsSessionRequiredError(err):
			return mcpclient.RuntimeStatus{}, response.NewBizError(http.StatusBadGateway, response.CodeServiceConnectFailed, "服务连接失败：session_mode=required，但服务端未返回会话", err)
		case mcpclient.IsSessionDisabledError(err):
			return mcpclient.RuntimeStatus{}, response.NewBizError(http.StatusBadGateway, response.CodeServiceConnectFailed, "服务连接失败：session_mode=disabled，但服务端返回了会话", err)
		default:
			return mcpclient.RuntimeStatus{}, response.NewBizError(http.StatusBadGateway, response.CodeServiceConnectFailed, "服务连接失败", err)
		}
	}
	_ = s.repo.UpdateStatus(ctx, id, entity.ServiceStatusConnected, 0, "")
	actor.Action = "connect_service"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = id
	actor.Detail = map[string]any{"transport_type": status.TransportType}
	_ = s.audit.Record(ctx, actor)
	return status, nil
}

// Disconnect 断开服务连接并更新持久化状态
func (s *mcpService) Disconnect(ctx context.Context, id string, actor AuditEntry) error {
	if err := s.connector.Disconnect(ctx, id); err != nil && err != mcpclient.ErrServiceNotConnected {
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

// Status 聚合数据库状态和运行时状态后返回
func (s *mcpService) Status(ctx context.Context, id string) (map[string]any, error) {
	service, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"id":                 service.ID,
		"name":               service.Name,
		"status":             service.Status,
		"persisted_status":   service.Status,
		"runtime_status":     nil,
		"status_source":      "persisted",
		"snapshot_freshness": "missing",
		"failure_count":      service.FailureCount,
		"last_error":         service.LastError,
		"transport_type":     service.TransportType,
	}
	if runtimeStatus, ok := s.statusReader.GetStatus(id); ok {
		out["status"] = runtimeStatus.Status
		out["runtime_status"] = runtimeStatus.Status
		out["status_source"] = "runtime"
		out["failure_count"] = runtimeStatus.FailureCount
		out["session_id_exists"] = runtimeStatus.SessionIDExists
		out["protocol_version"] = runtimeStatus.ProtocolVersion
		out["listen_enabled"] = runtimeStatus.ListenEnabled
		out["listen_active"] = runtimeStatus.ListenActive
		out["listen_last_error"] = runtimeStatus.ListenLastError
		out["last_seen_at"] = runtimeStatus.LastSeenAt
		out["last_used_at"] = runtimeStatus.LastUsedAt
		out["in_flight"] = runtimeStatus.InFlight
		out["transport_capabilities"] = runtimeStatus.TransportCapabilities
		out["transport_type"] = runtimeStatus.TransportType
		out["last_error"] = runtimeStatus.LastError
		return out, nil
	}
	snapshot, ok, err := s.runtimeStore.GetSnapshot(ctx, id)
	if err != nil {
		logger.S().Warn("读取运行态快照失败，回退 persisted 状态", "service_id", id, "error", err)
		return out, nil
	}
	if !ok {
		return out, nil
	}
	out["snapshot_observed_at"] = snapshot.ObservedAt
	if !s.isFreshSnapshot(snapshot) {
		out["snapshot_freshness"] = "stale"
		return out, nil
	}
	out["status"] = snapshot.Status
	out["status_source"] = "snapshot"
	out["snapshot_freshness"] = "fresh"
	out["failure_count"] = snapshot.FailureCount
	out["session_id_exists"] = snapshot.SessionIDExists
	out["protocol_version"] = snapshot.ProtocolVersion
	out["listen_enabled"] = snapshot.ListenEnabled
	out["listen_active"] = snapshot.ListenActive
	out["listen_last_error"] = snapshot.ListenLastError
	out["last_seen_at"] = snapshot.LastSeenAt
	out["last_used_at"] = snapshot.LastUsedAt
	out["in_flight"] = snapshot.InFlight
	out["transport_capabilities"] = snapshot.TransportCapabilities
	out["transport_type"] = snapshot.TransportType
	out["last_error"] = snapshot.LastError
	return out, nil
}

func (s *mcpService) isFreshSnapshot(snapshot mcpclient.RuntimeSnapshot) bool {
	if snapshot.ObservedAt.IsZero() || s.runtimeCfg.SnapshotTTL <= 0 {
		return false
	}
	return time.Since(snapshot.ObservedAt) <= s.runtimeCfg.SnapshotTTL
}

// buildServiceEntity 根据输入构建服务实体并执行校验
func buildServiceEntity(input CreateMCPServiceInput) (*entity.MCPService, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, response.NewBizError(http.StatusBadRequest, response.CodeInvalidArgument, "服务名称不能为空", nil)
	}
	if input.Timeout <= 0 {
		input.Timeout = 30
	}
	// 不同传输方式的必填字段不同，这里集中做协议级校验
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

// sanitizedServiceDetail 生成脱敏后的服务审计详情
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

// recordServiceError 记录服务错误事件并触发告警
func (s *mcpService) recordServiceError(ctx context.Context, service *entity.MCPService, actor AuditEntry, failureCount int, reason string, transition bool, source string) {
	// 进入错误态后主动断开连接，确保后续必须重新建立连接
	_ = s.connector.Disconnect(context.Background(), service.ID)
	if !transition {
		return
	}
	if actor.Username == "" {
		actor.Username = "system"
	}
	actor.Action = "service_error"
	actor.ResourceType = "mcp_service"
	actor.ResourceID = service.ID
	actor.Detail = map[string]any{
		"service_name":     service.Name,
		"transport_type":   service.TransportType,
		"status":           entity.ServiceStatusError,
		"failure_count":    failureCount,
		"reason":           reason,
		"source":           source,
		"listen_enabled":   service.ListenEnabled,
		"service_endpoint": serviceEndpoint(service),
	}
	_ = s.audit.Record(ctx, actor)
	_ = s.alerts.NotifyServiceError(ctx, service.Name, string(service.TransportType), serviceEndpoint(service), reason)
}

// serviceEndpoint 返回服务的主要访问端点
func serviceEndpoint(service *entity.MCPService) string {
	if service == nil {
		return ""
	}
	if service.URL != "" {
		return service.URL
	}
	return service.Command
}

// normalizeServiceRepoErr 将仓储错误转换为业务错误
func normalizeServiceRepoErr(err error) error {
	if err == nil {
		return nil
	}
	if err == repository.ErrAlreadyExists {
		return response.NewBizError(http.StatusConflict, response.CodeConflict, "服务名称已存在", err)
	}
	return err
}
