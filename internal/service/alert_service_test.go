package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/stretchr/testify/require"
)

// mockSendCall 记录一次邮件发送的参数
type mockSendCall struct {
	from, subject, body string
	to                  []string
}

// mockSender 实现 email.Sender 接口，用于测试
type mockSender struct {
	mu    sync.Mutex
	calls []mockSendCall
}

// Send 记录一次模拟邮件发送
func (m *mockSender) Send(from string, to []string, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, mockSendCall{from: from, to: to, subject: subject, body: body})
	return nil
}

// callCount 返回已记录的发送次数
func (m *mockSender) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// lastCall 返回最近一次发送记录
func (m *mockSender) lastCall() mockSendCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[len(m.calls)-1]
}

// TestAlertService_Disabled 验证告警未启用时不发送邮件
func TestAlertService_Disabled(t *testing.T) {
	sender := &mockSender{}
	svc := NewAlertService(config.AlertConfig{
		Enabled:       false,
		From:          "alert@example.com",
		To:            []string{"admin@example.com"},
		SubjectPrefix: "[TEST]",
		SilenceWindow: time.Minute,
	}, sender)

	err := svc.NotifyServiceError(context.Background(), "my-service", "sse", "http://localhost:8080", "connection refused")
	require.NoError(t, err)
	require.Equal(t, 0, sender.callCount())
}

// TestAlertService_NoRecipients 验证收件人为空时不发送邮件
func TestAlertService_NoRecipients(t *testing.T) {
	sender := &mockSender{}
	svc := NewAlertService(config.AlertConfig{
		Enabled:       true,
		From:          "alert@example.com",
		To:            []string{},
		SubjectPrefix: "[TEST]",
		SilenceWindow: time.Minute,
	}, sender)

	err := svc.NotifyServiceError(context.Background(), "my-service", "sse", "http://localhost:8080", "timeout")
	require.NoError(t, err)
	require.Equal(t, 0, sender.callCount())
}

// TestAlertService_SendsAlert 验证告警正常发送且邮件内容正确
func TestAlertService_SendsAlert(t *testing.T) {
	sender := &mockSender{}
	svc := NewAlertService(config.AlertConfig{
		Enabled:       true,
		From:          "alert@example.com",
		To:            []string{"admin@example.com", "ops@example.com"},
		SubjectPrefix: "[MCP]",
		SilenceWindow: time.Minute,
	}, sender)

	err := svc.NotifyServiceError(context.Background(), "payment-svc", "sse", "http://pay:9090", "connection refused")
	require.NoError(t, err)
	require.Equal(t, 1, sender.callCount())

	call := sender.lastCall()
	require.Equal(t, "alert@example.com", call.from)
	require.Equal(t, []string{"admin@example.com", "ops@example.com"}, call.to)
	require.Contains(t, call.subject, "[MCP]")
	require.Contains(t, call.subject, "payment-svc")
	require.Contains(t, call.body, "payment-svc")
	require.Contains(t, call.body, "sse")
	require.Contains(t, call.body, "http://pay:9090")
	require.Contains(t, call.body, "connection refused")
}

// TestAlertService_SilenceWindow 验证静默窗口内的重复告警被抑制，窗口过后可再次发送
func TestAlertService_SilenceWindow(t *testing.T) {
	sender := &mockSender{}
	// 使用极短的静默窗口方便测试
	silenceWindow := 100 * time.Millisecond
	svc := NewAlertService(config.AlertConfig{
		Enabled:       true,
		From:          "alert@example.com",
		To:            []string{"admin@example.com"},
		SubjectPrefix: "[TEST]",
		SilenceWindow: silenceWindow,
	}, sender)
	ctx := context.Background()

	// 第一次发送成功
	err := svc.NotifyServiceError(ctx, "db-svc", "stdio", "/usr/bin/db", "crash")
	require.NoError(t, err)
	require.Equal(t, 1, sender.callCount())

	// 在静默窗口内，同一服务的告警被抑制
	err = svc.NotifyServiceError(ctx, "db-svc", "stdio", "/usr/bin/db", "crash again")
	require.NoError(t, err)
	require.Equal(t, 1, sender.callCount()) // 仍然是 1

	// 不同服务名不受影响
	err = svc.NotifyServiceError(ctx, "cache-svc", "sse", "http://cache:6379", "timeout")
	require.NoError(t, err)
	require.Equal(t, 2, sender.callCount())

	// 等待静默窗口过去
	time.Sleep(silenceWindow + 50*time.Millisecond)

	// 窗口过后，同一服务可以再次发送
	err = svc.NotifyServiceError(ctx, "db-svc", "stdio", "/usr/bin/db", "crash once more")
	require.NoError(t, err)
	require.Equal(t, 3, sender.callCount())
}
