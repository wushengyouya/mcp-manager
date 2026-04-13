package mcpclient

import (
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/pkg/logger"
)

const defaultIdleReaperDryRunInterval = 30 * time.Second

// IdleReaperSkipReason 定义 dry-run 扫描未命中的原因。
type IdleReaperSkipReason string

const (
	IdleReaperSkipReasonFeatureDisabled     IdleReaperSkipReason = "feature_disabled"
	IdleReaperSkipReasonIdleTimeoutDisabled IdleReaperSkipReason = "idle_timeout_disabled"
	IdleReaperSkipReasonInFlight            IdleReaperSkipReason = "in_flight"
	IdleReaperSkipReasonListenEnabled       IdleReaperSkipReason = "listen_enabled"
	IdleReaperSkipReasonRecentlyUsed        IdleReaperSkipReason = "recently_used"
	IdleReaperSkipReasonNotConnected        IdleReaperSkipReason = "not_connected"
)

// IdleReaperDryRunObservation 定义单个连接的 dry-run 观测结果。
type IdleReaperDryRunObservation struct {
	ServiceID     string
	WouldReap     bool
	SkipReason    IdleReaperSkipReason
	IdleDuration  time.Duration
	LastUsedAt    *time.Time
	InFlight      int
	ListenEnabled bool
}

// EvaluateIdleReaperDryRun 根据当前运行态计算 dry-run 命中结果。
func EvaluateIdleReaperDryRun(status RuntimeStatus, cfg config.RuntimeConfig, now time.Time) IdleReaperDryRunObservation {
	observation := IdleReaperDryRunObservation{
		ServiceID:     status.ServiceID,
		LastUsedAt:    status.LastUsedAt,
		InFlight:      status.InFlight,
		ListenEnabled: status.ListenEnabled,
	}

	idleDuration, ok := status.IdleDurationAt(now)
	if ok {
		observation.IdleDuration = idleDuration
	}

	switch {
	case cfg.IdleTimeout <= 0:
		observation.SkipReason = IdleReaperSkipReasonIdleTimeoutDisabled
	case !cfg.IdleReaperDryRunEnabled:
		observation.SkipReason = IdleReaperSkipReasonFeatureDisabled
	case status.Status != entity.ServiceStatusConnected:
		observation.SkipReason = IdleReaperSkipReasonNotConnected
	case status.InFlight > 0:
		observation.SkipReason = IdleReaperSkipReasonInFlight
	case status.ListenEnabled:
		observation.SkipReason = IdleReaperSkipReasonListenEnabled
	case !ok || idleDuration < cfg.IdleTimeout:
		observation.SkipReason = IdleReaperSkipReasonRecentlyUsed
	default:
		observation.WouldReap = true
	}

	return observation
}

// IdleReaperDryRunScanner 定义周期性 idle dry-run 扫描器。
type IdleReaperDryRunScanner struct {
	manager   *Manager
	cfg       config.RuntimeConfig
	interval  time.Duration
	nowFn     func() time.Time
	observeFn func(IdleReaperDryRunObservation)
	stop      chan struct{}
}

// NewIdleReaperDryRunScanner 创建 dry-run idle 扫描器。
func NewIdleReaperDryRunScanner(manager *Manager, cfg config.RuntimeConfig) *IdleReaperDryRunScanner {
	return &IdleReaperDryRunScanner{
		manager:   manager,
		cfg:       cfg,
		interval:  defaultIdleReaperDryRunInterval,
		nowFn:     time.Now,
		observeFn: logIdleReaperDryRunObservation,
		stop:      make(chan struct{}),
	}
}

// Start 启动 dry-run idle 扫描。
func (s *IdleReaperDryRunScanner) Start() {
	if s.manager == nil {
		return
	}
	if s.interval <= 0 {
		s.interval = defaultIdleReaperDryRunInterval
	}
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.checkOnce()
			case <-s.stop:
				return
			}
		}
	}()
}

// Stop 停止 dry-run idle 扫描。
func (s *IdleReaperDryRunScanner) Stop() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
}

func (s *IdleReaperDryRunScanner) checkOnce() {
	s.manager.ScanIdleReaperDryRun(s.nowFn(), s.cfg, s.observeFn)
}

func logIdleReaperDryRunObservation(observation IdleReaperDryRunObservation) {
	logger.S().Infow(
		"idle dry-run 扫描",
		"service_id", observation.ServiceID,
		"would_reap", observation.WouldReap,
		"skip_reason", observation.SkipReason,
		"idle_duration", observation.IdleDuration.String(),
		"last_used_at", observation.LastUsedAt,
		"in_flight", observation.InFlight,
		"listen_enabled", observation.ListenEnabled,
	)
}
