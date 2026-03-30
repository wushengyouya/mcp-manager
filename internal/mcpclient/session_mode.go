package mcpclient

import (
	"errors"
	"fmt"
	"strings"

	transport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
)

const (
	sessionModeAuto     = "auto"
	sessionModeRequired = "required"
	sessionModeDisabled = "disabled"
)

// normalizeSessionMode 统一 session_mode 的默认值和大小写。
func normalizeSessionMode(raw string) string {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return sessionModeAuto
	}
	return mode
}

// validateSessionMode 校验 streamable_http 连接是否满足配置的会话策略。
func validateSessionMode(configuredTransport, actualTransport entity.TransportType, sessionMode string, hasSession bool) error {
	if configuredTransport != entity.TransportTypeStreamableHTTP {
		return nil
	}
	switch normalizeSessionMode(sessionMode) {
	case sessionModeRequired:
		if actualTransport != entity.TransportTypeStreamableHTTP {
			return ErrSessionRequired
		}
		if !hasSession {
			return ErrSessionRequired
		}
	case sessionModeDisabled:
		if actualTransport == entity.TransportTypeStreamableHTTP && hasSession {
			return ErrSessionDisabled
		}
	}
	return nil
}

// IsSessionReconnectRequired 判断错误是否表示会话已失效且必须重新连接。
func IsSessionReconnectRequired(err error) bool {
	return errors.Is(err, ErrSessionReinitializeRequired) || errors.Is(err, transport.ErrSessionTerminated)
}

// IsSessionRequiredError 判断错误是否来源于 required 模式校验失败。
func IsSessionRequiredError(err error) bool {
	return errors.Is(err, ErrSessionRequired)
}

// IsSessionDisabledError 判断错误是否来源于 disabled 模式校验失败。
func IsSessionDisabledError(err error) bool {
	return errors.Is(err, ErrSessionDisabled)
}

// wrapSessionReconnectRequired 将会话失效错误包装为统一重连错误。
func wrapSessionReconnectRequired(err error) error {
	if err == nil {
		return ErrSessionReinitializeRequired
	}
	if errors.Is(err, ErrSessionReinitializeRequired) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrSessionReinitializeRequired, err)
}
