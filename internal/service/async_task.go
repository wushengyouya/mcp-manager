package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// AsyncTaskStatus 定义异步任务状态。
type AsyncTaskStatus string

const (
	// AsyncTaskStatusPending 表示任务已入队待执行。
	AsyncTaskStatusPending AsyncTaskStatus = "pending"
	// AsyncTaskStatusRunning 表示任务执行中。
	AsyncTaskStatusRunning AsyncTaskStatus = "running"
	// AsyncTaskStatusSucceeded 表示任务执行成功。
	AsyncTaskStatusSucceeded AsyncTaskStatus = "succeeded"
	// AsyncTaskStatusFailed 表示任务执行失败。
	AsyncTaskStatusFailed AsyncTaskStatus = "failed"
	// AsyncTaskStatusCancelled 表示任务已取消。
	AsyncTaskStatusCancelled AsyncTaskStatus = "cancelled"
	// AsyncTaskStatusTimedOut 表示任务执行超时。
	AsyncTaskStatusTimedOut AsyncTaskStatus = "timed_out"
)

// AsyncInvokeTask 定义异步任务对外视图。
type AsyncInvokeTask struct {
	ID                string          `json:"id"`
	RequestID         string          `json:"request_id"`
	ToolID            string          `json:"tool_id"`
	ServiceID         string          `json:"service_id"`
	Status            AsyncTaskStatus `json:"status"`
	CancelRequested   bool            `json:"cancel_requested"`
	TimeoutMS         int             `json:"timeout_ms"`
	Result            map[string]any  `json:"result,omitempty"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	DurationMS        int64           `json:"duration_ms"`
	CreatedAt         time.Time       `json:"created_at"`
	StartedAt         *time.Time      `json:"started_at,omitempty"`
	FinishedAt        *time.Time      `json:"finished_at,omitempty"`
	QueueLength       int             `json:"queue_length"`
	QueueCapacity     int             `json:"queue_capacity"`
	ExecutorInFlight  int             `json:"executor_in_flight"`
	ExecutorLimit     int             `json:"executor_limit"`
	ServiceRateLimit  int             `json:"service_rate_limit"`
	UserRateLimit     int             `json:"user_rate_limit"`
	RateLimitWindowMS int64           `json:"rate_limit_window_ms"`
}

// AsyncTaskStats 定义任务总览。
type AsyncTaskStats struct {
	Pending           int   `json:"pending"`
	Running           int   `json:"running"`
	Succeeded         int   `json:"succeeded"`
	Failed            int   `json:"failed"`
	Cancelled         int   `json:"cancelled"`
	TimedOut          int   `json:"timed_out"`
	QueueLength       int   `json:"queue_length"`
	QueueCapacity     int   `json:"queue_capacity"`
	ExecutorInFlight  int   `json:"executor_in_flight"`
	ExecutorLimit     int   `json:"executor_limit"`
	ServiceRateLimit  int   `json:"service_rate_limit"`
	UserRateLimit     int   `json:"user_rate_limit"`
	RateLimitWindowMS int64 `json:"rate_limit_window_ms"`
}

// AsyncInvokeService 定义异步任务能力。
type AsyncInvokeService interface {
	InvokeAsync(ctx context.Context, toolID string, arguments map[string]any, timeout time.Duration, actor AuditEntry) (*AsyncInvokeTask, error)
	GetTask(ctx context.Context, taskID string, actor AuditEntry) (*AsyncInvokeTask, error)
	CancelTask(ctx context.Context, taskID string, actor AuditEntry) (*AsyncInvokeTask, error)
	TaskStats(ctx context.Context, actor AuditEntry) (*AsyncTaskStats, error)
}

type asyncTaskManager struct {
	controller     *InvokeController
	defaultTimeout time.Duration
	queue          chan string
	mu             sync.RWMutex
	tasks          map[string]*asyncTaskEntry
	wg             sync.WaitGroup
	closed         bool
}

type asyncTaskEntry struct {
	task   AsyncInvokeTask
	userID string
	role   entity.Role
	run    func(context.Context) (*ToolInvokeResult, error)
	ctx    context.Context
	cancel context.CancelFunc
}

func newAsyncTaskManager(cfg config.ExecutionConfig, controller *InvokeController) *asyncTaskManager {
	workers := cfg.AsyncTaskWorkers
	if workers <= 0 {
		workers = 1
	}
	capacity := cfg.AsyncTaskQueueSize
	if capacity <= 0 {
		capacity = 1
	}
	m := &asyncTaskManager{
		controller:     controller,
		defaultTimeout: cfg.DefaultTaskTimeout,
		queue:          make(chan string, capacity),
		tasks:          map[string]*asyncTaskEntry{},
	}
	m.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go m.worker()
	}
	return m
}

func (m *asyncTaskManager) submit(toolID, serviceID string, arguments map[string]any, timeout time.Duration, actor AuditEntry, run func(context.Context) (*ToolInvokeResult, error)) (*AsyncInvokeTask, error) {
	if m == nil {
		return nil, response.NewBizError(http.StatusNotImplemented, response.CodeConflict, "异步调用未启用", nil)
	}
	if timeout <= 0 {
		timeout = m.defaultTimeout
	}
	requestID := uuid.NewString()
	taskCtx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		taskCtx, cancel = context.WithTimeout(taskCtx, timeout)
	} else {
		taskCtx, cancel = context.WithCancel(taskCtx)
	}
	entry := &asyncTaskEntry{
		task: AsyncInvokeTask{
			ID:        uuid.NewString(),
			RequestID: requestID,
			ToolID:    toolID,
			ServiceID: serviceID,
			Status:    AsyncTaskStatusPending,
			TimeoutMS: int(timeout.Milliseconds()),
			CreatedAt: time.Now(),
		},
		userID: actor.UserID,
		role:   actor.Role,
		run:    run,
		ctx:    taskCtx,
		cancel: cancel,
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		cancel()
		return nil, response.NewBizError(http.StatusServiceUnavailable, response.CodeConflict, "异步任务队列已停止", nil)
	}
	m.tasks[entry.task.ID] = entry
	entry.task.QueueLength = len(m.queue) + 1
	entry.task.QueueCapacity = cap(m.queue)
	entry.task.ExecutorLimit = m.controller.Stats().ExecutorLimit
	entry.task.ExecutorInFlight = m.controller.Stats().InFlight
	entry.task.ServiceRateLimit = m.controller.Stats().ServiceRateLimit
	entry.task.UserRateLimit = m.controller.Stats().UserRateLimit
	entry.task.RateLimitWindowMS = m.controller.Stats().RateLimitWindow.Milliseconds()
	m.mu.Unlock()

	select {
	case m.queue <- entry.task.ID:
		return m.snapshot(entry.task.ID), nil
	default:
		m.mu.Lock()
		delete(m.tasks, entry.task.ID)
		m.mu.Unlock()
		cancel()
		return nil, response.NewBizError(http.StatusTooManyRequests, response.CodeTooManyRequests, "异步任务队列已满", ErrAsyncQueueFull)
	}
}

func (m *asyncTaskManager) worker() {
	defer m.wg.Done()
	for id := range m.queue {
		entry := m.getEntry(id)
		if entry == nil {
			continue
		}
		if m.isTerminal(entry.task.Status) {
			continue
		}
		startedAt := time.Now()
		m.update(id, func(task *AsyncInvokeTask) {
			task.Status = AsyncTaskStatusRunning
			task.StartedAt = &startedAt
			task.QueueLength = len(m.queue)
		})

		release, err := m.controller.WaitAcquire(entry.ctx)
		if err != nil {
			m.finish(id, nil, err)
			continue
		}
		result, execErr := entry.run(entry.ctx)
		release()
		m.finish(id, result, execErr)
	}
}

func (m *asyncTaskManager) finish(id string, result *ToolInvokeResult, err error) {
	finishedAt := time.Now()
	m.update(id, func(task *AsyncInvokeTask) {
		task.FinishedAt = &finishedAt
		if task.StartedAt != nil {
			task.DurationMS = finishedAt.Sub(*task.StartedAt).Milliseconds()
		}
		stats := m.controller.Stats()
		task.QueueLength = len(m.queue)
		task.QueueCapacity = cap(m.queue)
		task.ExecutorInFlight = stats.InFlight
		task.ExecutorLimit = stats.ExecutorLimit
		task.ServiceRateLimit = stats.ServiceRateLimit
		task.UserRateLimit = stats.UserRateLimit
		task.RateLimitWindowMS = stats.RateLimitWindow.Milliseconds()
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			task.Status = AsyncTaskStatusTimedOut
			task.ErrorMessage = "任务执行超时"
		case errors.Is(err, context.Canceled):
			task.Status = AsyncTaskStatusCancelled
			task.ErrorMessage = "任务已取消"
		case err != nil:
			task.Status = AsyncTaskStatusFailed
			task.ErrorMessage = err.Error()
		default:
			task.Status = AsyncTaskStatusSucceeded
			task.ErrorMessage = ""
			if result != nil {
				task.Result = result.Result
				task.DurationMS = result.DurationMS
			}
		}
	})
}

func (m *asyncTaskManager) snapshot(taskID string) *AsyncInvokeTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry := m.tasks[taskID]
	if entry == nil {
		return nil
	}
	cloned := entry.task
	return &cloned
}

func (m *asyncTaskManager) getEntry(taskID string) *asyncTaskEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tasks[taskID]
}

func (m *asyncTaskManager) update(taskID string, apply func(*AsyncInvokeTask)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.tasks[taskID]
	if entry == nil || apply == nil {
		return
	}
	apply(&entry.task)
}

func (m *asyncTaskManager) get(taskID string, actor AuditEntry) (*AsyncInvokeTask, error) {
	entry := m.getEntry(taskID)
	if entry == nil {
		return nil, response.NewBizError(http.StatusNotFound, response.CodeNotFound, "任务不存在", nil)
	}
	if !canAccessTask(entry, actor) {
		return nil, response.NewBizError(http.StatusForbidden, response.CodeForbidden, "权限不足", nil)
	}
	return m.snapshot(taskID), nil
}

func (m *asyncTaskManager) cancel(taskID string, actor AuditEntry) (*AsyncInvokeTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.tasks[taskID]
	if entry == nil {
		return nil, response.NewBizError(http.StatusNotFound, response.CodeNotFound, "任务不存在", nil)
	}
	if !canAccessTask(entry, actor) {
		return nil, response.NewBizError(http.StatusForbidden, response.CodeForbidden, "权限不足", nil)
	}
	if m.isTerminal(entry.task.Status) {
		cloned := entry.task
		return &cloned, nil
	}
	entry.task.CancelRequested = true
	if entry.task.Status == AsyncTaskStatusPending {
		entry.task.Status = AsyncTaskStatusCancelled
		now := time.Now()
		entry.task.FinishedAt = &now
	}
	entry.cancel()
	cloned := entry.task
	return &cloned, nil
}

func (m *asyncTaskManager) stats(actor AuditEntry) (*AsyncTaskStats, error) {
	if actor.Role != entity.RoleAdmin {
		return nil, response.NewBizError(http.StatusForbidden, response.CodeForbidden, "权限不足", nil)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := &AsyncTaskStats{}
	for _, entry := range m.tasks {
		switch entry.task.Status {
		case AsyncTaskStatusPending:
			stats.Pending++
		case AsyncTaskStatusRunning:
			stats.Running++
		case AsyncTaskStatusSucceeded:
			stats.Succeeded++
		case AsyncTaskStatusFailed:
			stats.Failed++
		case AsyncTaskStatusCancelled:
			stats.Cancelled++
		case AsyncTaskStatusTimedOut:
			stats.TimedOut++
		}
	}
	control := m.controller.Stats()
	stats.QueueLength = len(m.queue)
	stats.QueueCapacity = cap(m.queue)
	stats.ExecutorInFlight = control.InFlight
	stats.ExecutorLimit = control.ExecutorLimit
	stats.ServiceRateLimit = control.ServiceRateLimit
	stats.UserRateLimit = control.UserRateLimit
	stats.RateLimitWindowMS = control.RateLimitWindow.Milliseconds()
	return stats, nil
}

func (m *asyncTaskManager) stop(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	close(m.queue)
	m.mu.Unlock()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	if ctx == nil {
		<-done
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *asyncTaskManager) isTerminal(status AsyncTaskStatus) bool {
	switch status {
	case AsyncTaskStatusSucceeded, AsyncTaskStatusFailed, AsyncTaskStatusCancelled, AsyncTaskStatusTimedOut:
		return true
	default:
		return false
	}
}

func canAccessTask(entry *asyncTaskEntry, actor AuditEntry) bool {
	if entry == nil {
		return false
	}
	if actor.Role == entity.RoleAdmin {
		return true
	}
	return actor.UserID != "" && actor.UserID == entry.userID
}
