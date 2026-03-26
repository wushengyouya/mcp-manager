package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
	"github.com/stretchr/testify/require"
)

type auditRecorder struct {
	entries []AuditEntry
}

func (a *auditRecorder) Record(_ context.Context, entry AuditEntry) error {
	a.entries = append(a.entries, entry)
	return nil
}

type alertRecorder struct {
	calls []struct {
		serviceName   string
		transportType string
		endpoint      string
		reason        string
	}
}

func (a *alertRecorder) NotifyServiceError(_ context.Context, serviceName, transportType, endpoint, reason string) error {
	a.calls = append(a.calls, struct {
		serviceName   string
		transportType string
		endpoint      string
		reason        string
	}{serviceName: serviceName, transportType: transportType, endpoint: endpoint, reason: reason})
	return nil
}

func TestBuildServiceEntityDefaultsAndValidation(t *testing.T) {
	_, err := buildServiceEntity(CreateMCPServiceInput{})
	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, http.StatusBadRequest, bizErr.HTTPStatus)

	_, err = buildServiceEntity(CreateMCPServiceInput{
		Name:          "stdio-svc",
		TransportType: entity.TransportTypeStdio,
	})
	require.ErrorAs(t, err, &bizErr)
	require.Contains(t, bizErr.Message, "command")

	_, err = buildServiceEntity(CreateMCPServiceInput{
		Name:          "http-svc",
		TransportType: entity.TransportTypeStreamableHTTP,
	})
	require.ErrorAs(t, err, &bizErr)
	require.Contains(t, bizErr.Message, "url")

	_, err = buildServiceEntity(CreateMCPServiceInput{
		Name:          "invalid-svc",
		TransportType: "invalid",
	})
	require.ErrorAs(t, err, &bizErr)
	require.Contains(t, bizErr.Message, "transport_type")

	item, err := buildServiceEntity(CreateMCPServiceInput{
		Name:          "http-svc",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc.test/mcp",
	})
	require.NoError(t, err)
	require.Equal(t, 30, item.Timeout)
	require.Equal(t, "auto", item.SessionMode)
	require.Equal(t, "off", item.CompatMode)
	require.Equal(t, entity.ServiceStatusDisconnected, item.Status)
}

func TestSanitizedServiceDetailAndHelpers(t *testing.T) {
	item := &entity.MCPService{
		Name:          "svc-mask",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc.test/mcp",
		Command:       "echo",
		BearerToken:   "secret",
		CustomHeaders: entity.JSONStringMap{
			"X-Test":        "1",
			"Authorization": "Bearer token",
			"X-Secret-Key":  "hidden",
		},
		SessionMode:   "required",
		CompatMode:    "allow_legacy_sse",
		ListenEnabled: true,
	}
	detail := sanitizedServiceDetail(item)
	headers := detail["custom_headers"].(map[string]string)
	require.Equal(t, "***", headers["Authorization"])
	require.Equal(t, "***", headers["X-Secret-Key"])
	require.Equal(t, "1", headers["X-Test"])
	require.Equal(t, "***", detail["bearer_token"])

	require.Equal(t, "http://svc.test/mcp", serviceEndpoint(item))
	require.Equal(t, "echo", serviceEndpoint(&entity.MCPService{Command: "echo"}))
	require.Equal(t, "", serviceEndpoint(nil))

	require.Nil(t, normalizeServiceRepoErr(nil))
	err := normalizeServiceRepoErr(repository.ErrAlreadyExists)
	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeConflict, bizErr.Code)
}

func TestMCPServiceUpdateGetListDisconnectAndStatus(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _, manager := setupMCPServiceTest(t)
	auditSink := &auditRecorder{}
	alertSvc := &alertRecorder{}
	svc := NewMCPService(serviceRepo, toolRepo, manager, auditSink, alertSvc)

	created, err := svc.Create(ctx, CreateMCPServiceInput{
		Name:          "svc-main",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-main.test/mcp",
		BearerToken:   "secret-token",
	}, AuditEntry{UserID: "u-1", Username: "root"})
	require.NoError(t, err)

	updated, err := svc.Update(ctx, created.ID, CreateMCPServiceInput{
		Name:          "svc-main",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-main.test/v2",
	}, AuditEntry{UserID: "u-1", Username: "root"})
	require.NoError(t, err)
	require.Equal(t, "secret-token", updated.BearerToken)
	require.Equal(t, "http://svc-main.test/v2", updated.URL)

	got, err := svc.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, updated.ID, got.ID)

	items, total, err := svc.List(ctx, repository.MCPServiceListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)

	require.NoError(t, svc.Disconnect(ctx, created.ID, AuditEntry{UserID: "u-1", Username: "root"}))
	status, err := svc.Status(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ServiceStatusDisconnected, status["status"])
}

func TestMCPServiceConnectUnsupportedTransportMarksError(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _, manager := setupMCPServiceTest(t)
	auditSink := &auditRecorder{}
	alertSvc := &alertRecorder{}
	svc := NewMCPService(serviceRepo, toolRepo, manager, auditSink, alertSvc)

	item := &entity.MCPService{
		Name:          "svc-invalid",
		TransportType: entity.TransportType("invalid"),
		Status:        entity.ServiceStatusDisconnected,
	}
	require.NoError(t, serviceRepo.Create(ctx, item))

	_, err := svc.Connect(ctx, item.ID, AuditEntry{Username: "root"})
	var bizErr *response.BizError
	require.ErrorAs(t, err, &bizErr)
	require.Equal(t, response.CodeServiceConnectFailed, bizErr.Code)

	updated, repoErr := serviceRepo.GetByID(ctx, item.ID)
	require.NoError(t, repoErr)
	require.Equal(t, entity.ServiceStatusError, updated.Status)
	require.Equal(t, 1, updated.FailureCount)
	require.Len(t, auditSink.entries, 1)
	require.Len(t, alertSvc.calls, 1)
}

func TestMCPServiceRecordServiceErrorTransition(t *testing.T) {
	manager := mcpclient.NewManager(config.AppConfig{})
	auditSink := &auditRecorder{}
	alertSvc := &alertRecorder{}
	svc := &mcpService{
		manager: manager,
		audit:   auditSink,
		alerts:  alertSvc,
	}
	item := &entity.MCPService{
		Base:          entity.Base{ID: "svc-record"},
		Name:          "svc-record",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-record.test/mcp",
	}

	svc.recordServiceError(context.Background(), item, AuditEntry{}, 2, "boom", true, "connect")
	require.Len(t, auditSink.entries, 1)
	require.Equal(t, "system", auditSink.entries[0].Username)
	require.Len(t, alertSvc.calls, 1)

	svc.recordServiceError(context.Background(), item, AuditEntry{Username: "operator"}, 3, "still boom", false, "connect")
	require.Len(t, auditSink.entries, 1)
	require.Len(t, alertSvc.calls, 1)
}

func TestToolServiceListByServiceAndGet(t *testing.T) {
	ctx := context.Background()
	serviceRepo, toolRepo, _, manager := setupMCPServiceTest(t)
	serviceItem := &entity.MCPService{
		Name:          "svc-tools",
		TransportType: entity.TransportTypeStreamableHTTP,
		URL:           "http://svc-tools.test/mcp",
	}
	require.NoError(t, serviceRepo.Create(ctx, serviceItem))
	require.NoError(t, toolRepo.Create(ctx, &entity.Tool{
		MCPServiceID: serviceItem.ID,
		Name:         "search",
		Description:  "search tool",
		InputSchema:  entity.JSONMap{"type": "object"},
		IsEnabled:    true,
		SyncedAt:     time.Now(),
	}))

	svc := NewToolService(toolRepo, serviceRepo, manager, nil)
	items, err := svc.ListByService(ctx, serviceItem.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)

	item, err := svc.Get(ctx, items[0].ID)
	require.NoError(t, err)
	require.Equal(t, "search", item.Name)
}

func TestSanitizeMapAndDeepSanitize(t *testing.T) {
	input := map[string]any{
		"token":         "secret",
		"password":      "hidden",
		"authorization": "Bearer secret",
		"nested": map[string]any{
			"apiToken": "nested-secret",
			"value":    "keep",
		},
	}

	clean, truncated, hash, size := sanitizeMap(input, 4096)
	require.False(t, truncated)
	require.NotEmpty(t, hash)
	require.Greater(t, size, 0)
	require.Equal(t, "***", clean["token"])
	require.Equal(t, "***", clean["password"])
	require.Equal(t, "***", clean["authorization"])
	require.Equal(t, "***", clean["nested"].(map[string]any)["apiToken"])
	require.Equal(t, "keep", clean["nested"].(map[string]any)["value"])

	truncatedMap, truncated, hash, size := sanitizeMap(map[string]any{"payload": string(make([]byte, 128))}, 8)
	require.True(t, truncated)
	require.NotEmpty(t, hash)
	require.Greater(t, size, 8)
	require.NotNil(t, truncatedMap)

	require.Equal(t, map[string]any{}, deepSanitize(nil))
	require.True(t, isMap(map[string]any{"ok": true}))
	require.False(t, isMap([]string{"not-map"}))
}
