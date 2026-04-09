package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
)

var (
	// ErrInvokeConcurrencyLimited 表示执行并发已达上限。
	ErrInvokeConcurrencyLimited = errors.New("invoke concurrency limited")
	// ErrInvokeRateLimited 表示请求命中了速率限制。
	ErrInvokeRateLimited = errors.New("invoke rate limited")
)

// InvokeLimitScope 描述限流命中维度。
type InvokeLimitScope string

const (
	// InvokeLimitScopeService 表示服务级限流。
	InvokeLimitScopeService InvokeLimitScope = "service"
	// InvokeLimitScopeUser 表示用户级限流。
	InvokeLimitScopeUser InvokeLimitScope = "user"
)

// InvokeLimitDecision 描述一次限流判定结果。
type InvokeLimitDecision struct {
	Allowed bool
	Scope   InvokeLimitScope
	Key     string
	Limit   int
	Window  time.Duration
	Reason  string
	Count   int
	At      time.Time
}

// InvokeControlStats 描述当前执行治理快照。
type InvokeControlStats struct {
	InFlight         int           `json:"in_flight"`
	ExecutorLimit    int           `json:"executor_limit"`
	RateLimitWindow  time.Duration `json:"rate_limit_window"`
	ServiceRateLimit int           `json:"service_rate_limit"`
	UserRateLimit    int           `json:"user_rate_limit"`
}

// InvokeController 封装执行并发与限流控制。
type InvokeController struct {
	concurrencyLimit int
	serviceLimit     int
	userLimit        int
	window           time.Duration

	sem chan struct{}

	mu      sync.Mutex
	service map[string]*invokeBucket
	user    map[string]*invokeBucket
}

type invokeBucket struct {
	windowStart time.Time
	count       int
}

// NewInvokeController 创建执行治理控制器。
func NewInvokeController(cfg config.ExecutionConfig) *InvokeController {
	controller := &InvokeController{
		concurrencyLimit: cfg.ExecutorConcurrency,
		serviceLimit:     cfg.ServiceRateLimit,
		userLimit:        cfg.UserRateLimit,
		window:           cfg.RateLimitWindow,
		service:          map[string]*invokeBucket{},
		user:             map[string]*invokeBucket{},
	}
	if controller.window <= 0 {
		controller.window = time.Minute
	}
	if controller.concurrencyLimit > 0 {
		controller.sem = make(chan struct{}, controller.concurrencyLimit)
	}
	return controller
}

// Acquire 尝试占用一个执行槽位；超限时快速失败，不阻塞请求线程。
func (c *InvokeController) Acquire(_ context.Context) (func(), error) {
	if c == nil || c.sem == nil {
		return func() {}, nil
	}
	select {
	case c.sem <- struct{}{}:
		return func() { <-c.sem }, nil
	default:
		return nil, ErrInvokeConcurrencyLimited
	}
}

// WaitAcquire 等待占用执行槽位；适合后台任务消费。
func (c *InvokeController) WaitAcquire(ctx context.Context) (func(), error) {
	if c == nil || c.sem == nil {
		return func() {}, nil
	}
	select {
	case c.sem <- struct{}{}:
		return func() { <-c.sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Allow 判断服务级与用户级限流是否命中。
func (c *InvokeController) Allow(serviceID, userID string) InvokeLimitDecision {
	if c == nil {
		return InvokeLimitDecision{Allowed: true, At: time.Now()}
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if decision := c.allowLocked(c.service, InvokeLimitScopeService, serviceID, c.serviceLimit, now); !decision.Allowed {
		return decision
	}
	if decision := c.allowLocked(c.user, InvokeLimitScopeUser, userID, c.userLimit, now); !decision.Allowed {
		return decision
	}
	return InvokeLimitDecision{Allowed: true, At: now, Window: c.window, Limit: 0}
}

func (c *InvokeController) allowLocked(store map[string]*invokeBucket, scope InvokeLimitScope, key string, limit int, now time.Time) InvokeLimitDecision {
	if limit <= 0 || key == "" {
		return InvokeLimitDecision{Allowed: true, At: now, Window: c.window, Scope: scope, Key: key, Limit: limit}
	}
	bucket := store[key]
	if bucket == nil || now.Sub(bucket.windowStart) >= c.window {
		bucket = &invokeBucket{windowStart: now}
		store[key] = bucket
	}
	bucket.count++
	if bucket.count > limit {
		return InvokeLimitDecision{
			Allowed: false,
			Scope:   scope,
			Key:     key,
			Limit:   limit,
			Window:  c.window,
			Reason:  fmt.Sprintf("%s 维度限流命中", scope),
			Count:   bucket.count,
			At:      now,
		}
	}
	return InvokeLimitDecision{Allowed: true, Scope: scope, Key: key, Limit: limit, Window: c.window, Count: bucket.count, At: now}
}

// Stats 返回当前执行治理快照。
func (c *InvokeController) Stats() InvokeControlStats {
	if c == nil {
		return InvokeControlStats{}
	}
	stats := InvokeControlStats{
		ExecutorLimit:    c.concurrencyLimit,
		RateLimitWindow:  c.window,
		ServiceRateLimit: c.serviceLimit,
		UserRateLimit:    c.userLimit,
	}
	if c.sem != nil {
		stats.InFlight = len(c.sem)
	}
	return stats
}
