package service

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupAuditTest 初始化内存数据库、仓储和审计相关服务。
func setupAuditTest(t *testing.T) (AuditService, AuditSink, repository.AuditLogRepository, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&entity.AuditLog{}))

	auditRepo := repository.NewAuditLogRepository(db)
	sink := NewDBAuditSink(auditRepo)
	svc := NewAuditService(sink, auditRepo)
	return svc, sink, auditRepo, db
}

// TestNoopAuditSink_Record 验证 NoopAuditSink 始终返回 nil。
func TestNoopAuditSink_Record(t *testing.T) {
	noop := NoopAuditSink{}
	err := noop.Record(context.Background(), AuditEntry{
		UserID:       "uid",
		Username:     "user",
		Action:       "test",
		ResourceType: "some",
	})
	require.NoError(t, err)
}

// TestDBAuditSink_Record 验证 DBAuditSink 将审计日志写入数据库。
func TestDBAuditSink_Record(t *testing.T) {
	_, sink, auditRepo, _ := setupAuditTest(t)
	ctx := context.Background()

	err := sink.Record(ctx, AuditEntry{
		UserID:       "user-123",
		Username:     "admin",
		Action:       "create_user",
		ResourceType: "user",
		ResourceID:   "new-user-456",
		Detail:       map[string]any{"username": "newguy"},
		IPAddress:    "10.0.0.1",
		UserAgent:    "test-agent",
	})
	require.NoError(t, err)

	// 从数据库读取确认已写入
	items, total, err := auditRepo.List(ctx, repository.AuditListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, "user-123", items[0].UserID)
	require.Equal(t, "admin", items[0].Username)
	require.Equal(t, "create_user", items[0].Action)
	require.Equal(t, "user", items[0].ResourceType)
	require.Equal(t, "new-user-456", items[0].ResourceID)
}

// TestAuditService_Record 验证 AuditService.Record 代理到 Sink。
func TestAuditService_Record(t *testing.T) {
	svc, _, auditRepo, _ := setupAuditTest(t)
	ctx := context.Background()

	err := svc.Record(ctx, AuditEntry{
		UserID:       "uid",
		Username:     "operator",
		Action:       "login",
		ResourceType: "auth",
		IPAddress:    "192.168.1.1",
	})
	require.NoError(t, err)

	items, total, err := auditRepo.List(ctx, repository.AuditListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "login", items[0].Action)
}

// TestAuditService_List 验证审计日志列表查询与过滤。
func TestAuditService_List(t *testing.T) {
	svc, _, _, _ := setupAuditTest(t)
	ctx := context.Background()

	// 写入多条审计记录
	entries := []AuditEntry{
		{UserID: "u1", Username: "alice", Action: "login", ResourceType: "auth", IPAddress: "1.1.1.1"},
		{UserID: "u2", Username: "bob", Action: "create_user", ResourceType: "user", IPAddress: "2.2.2.2"},
		{UserID: "u1", Username: "alice", Action: "logout", ResourceType: "auth", IPAddress: "1.1.1.1"},
	}
	for _, e := range entries {
		require.NoError(t, svc.Record(ctx, e))
	}

	// 列出所有
	items, total, err := svc.List(ctx, repository.AuditListFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, items, 3)

	// 按 Action 过滤
	items, total, err = svc.List(ctx, repository.AuditListFilter{Page: 1, PageSize: 10, Action: "login"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, "login", items[0]["action"])

	// 按 UserID 过滤
	items, total, err = svc.List(ctx, repository.AuditListFilter{Page: 1, PageSize: 10, UserID: "u1"})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, items, 2)

	// 按 ResourceType 过滤
	items, total, err = svc.List(ctx, repository.AuditListFilter{Page: 1, PageSize: 10, ResourceType: "user"})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "create_user", items[0]["action"])
}

// TestAuditService_ExportCSV 验证导出 CSV 的表头和数据行。
func TestAuditService_ExportCSV(t *testing.T) {
	svc, _, _, _ := setupAuditTest(t)
	ctx := context.Background()

	// 写入测试数据
	require.NoError(t, svc.Record(ctx, AuditEntry{
		UserID:       "u1",
		Username:     "alice",
		Action:       "login",
		ResourceType: "auth",
		ResourceID:   "",
		Detail:       map[string]any{"ip": "1.1.1.1"},
		IPAddress:    "1.1.1.1",
	}))
	require.NoError(t, svc.Record(ctx, AuditEntry{
		UserID:       "u2",
		Username:     "bob",
		Action:       "create_user",
		ResourceType: "user",
		ResourceID:   "new-id",
		Detail:       map[string]any{"username": "charlie"},
		IPAddress:    "2.2.2.2",
	}))

	data, err := svc.ExportCSV(ctx, repository.AuditListFilter{Page: 1, PageSize: 100})
	require.NoError(t, err)
	require.NotEmpty(t, data)

	reader := csv.NewReader(strings.NewReader(string(data)))
	records, err := reader.ReadAll()
	require.NoError(t, err)

	// 表头 + 2 行数据
	require.Len(t, records, 3)

	// 验证表头
	header := records[0]
	require.Equal(t, []string{"id", "username", "action", "resource_type", "resource_id", "detail", "created_at"}, header)

	// 验证数据行包含预期字段（按 created_at desc 排序，最新在前）
	require.Equal(t, "bob", records[1][1])
	require.Equal(t, "create_user", records[1][2])
	require.Equal(t, "alice", records[2][1])
	require.Equal(t, "login", records[2][2])
}
