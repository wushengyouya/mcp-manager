package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/pkg/email"
)

// AlertService 定义告警服务。
type AlertService interface {
	NotifyServiceError(ctx context.Context, serviceName, transportType, endpoint, reason string) error
}

type noopAlertService struct{}

func (noopAlertService) NotifyServiceError(context.Context, string, string, string, string) error {
	return nil
}

type alertService struct {
	cfg    config.AlertConfig
	sender email.Sender
	mu     sync.Mutex
	last   map[string]time.Time
}

// NewAlertService 创建告警服务。
func NewAlertService(cfg config.AlertConfig, sender email.Sender) AlertService {
	return &alertService{cfg: cfg, sender: sender, last: make(map[string]time.Time)}
}

func (s *alertService) NotifyServiceError(ctx context.Context, serviceName, transportType, endpoint, reason string) error {
	if !s.cfg.Enabled || s.sender == nil || len(s.cfg.To) == 0 {
		return nil
	}
	s.mu.Lock()
	last := s.last[serviceName]
	if !last.IsZero() && time.Since(last) < s.cfg.SilenceWindow {
		s.mu.Unlock()
		return nil
	}
	s.last[serviceName] = time.Now()
	s.mu.Unlock()

	subject := fmt.Sprintf("%s 服务告警: %s", s.cfg.SubjectPrefix, serviceName)
	body := fmt.Sprintf("服务名称: %s\n传输方式: %s\n端点: %s\n错误: %s\n时间: %s\n", serviceName, transportType, endpoint, reason, time.Now().Format(time.RFC3339))
	return s.sender.Send(s.cfg.From, s.cfg.To, subject, body)
}
