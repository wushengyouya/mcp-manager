package service

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
)

// AuthStateManager 聚合 token_version、会话状态和撤销逻辑。
type AuthStateManager struct {
	users         repository.UserRepository
	sessions      repository.AuthSessionRepository
	userVersions  UserTokenVersionStore
	sessionStates SessionStateStore
}

// NewAuthStateManager 创建认证状态管理器。
func NewAuthStateManager(users repository.UserRepository, sessions repository.AuthSessionRepository, userVersions UserTokenVersionStore, sessionStates SessionStateStore) *AuthStateManager {
	if userVersions == nil {
		userVersions = NoopUserTokenVersionStore{}
	}
	if sessionStates == nil {
		sessionStates = NoopSessionStateStore{}
	}
	return &AuthStateManager{users: users, sessions: sessions, userVersions: userVersions, sessionStates: sessionStates}
}

// ValidateAccessToken 校验 access token 的版本号与会话状态。
func (m *AuthStateManager) ValidateAccessToken(ctx context.Context, claims *appcrypto.AccessClaims) error {
	if claims == nil {
		return jwt.ErrTokenMalformed
	}
	version, err := m.currentTokenVersion(ctx, claims.UserID)
	if err != nil {
		return err
	}
	if version != claims.TokenVersion {
		return appcrypto.ErrTokenVersionMismatch
	}
	if claims.SessionID == "" {
		return nil
	}
	status, err := m.currentSessionStatus(ctx, claims.SessionID)
	if err != nil {
		return err
	}
	if status != entity.AuthSessionStatusActive {
		return appcrypto.ErrSessionRevoked
	}
	return nil
}

// RevokeAllForUser 递增 token_version 并撤销用户所有活跃会话。
func (m *AuthStateManager) RevokeAllForUser(ctx context.Context, userID, reason string) (int64, error) {
	if m == nil || m.users == nil {
		return 0, nil
	}
	version, err := m.users.BumpTokenVersion(ctx, userID)
	if err != nil {
		return 0, err
	}
	if err := m.userVersions.SetUserTokenVersion(ctx, userID, version); err != nil {
		return 0, err
	}
	if err := m.RevokeSessionsForUser(ctx, userID, reason); err != nil {
		return 0, err
	}
	return version, nil
}

// RevokeSessionsForUser 撤销用户所有活跃会话但不修改 token_version。
func (m *AuthStateManager) RevokeSessionsForUser(ctx context.Context, userID, reason string) error {
	if m == nil || m.sessions == nil {
		return nil
	}
	now := time.Now()
	sessions, err := m.sessions.ListActiveByUserID(ctx, userID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}
	if _, err := m.sessions.RevokeByUserID(ctx, userID, reason, now); err != nil {
		return err
	}
	for _, session := range sessions {
		cacheSetSessionStatus(ctx, m.sessionStates, session.ID, entity.AuthSessionStatusRevoked, session.ExpiresAt)
	}
	return nil
}

// RevokeSession 撤销指定会话并同步缓存。
func (m *AuthStateManager) RevokeSession(ctx context.Context, sessionID, reason string) error {
	if m == nil || m.sessions == nil || sessionID == "" {
		return nil
	}
	session, err := m.sessions.GetByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	if err := m.sessions.RevokeByID(ctx, sessionID, reason, time.Now()); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return err
	}
	cacheSetSessionStatus(ctx, m.sessionStates, sessionID, entity.AuthSessionStatusRevoked, session.ExpiresAt)
	return nil
}

// MarkSessionStatus 同步会话状态到缓存。
func (m *AuthStateManager) MarkSessionStatus(ctx context.Context, sessionID string, status entity.AuthSessionStatus, expireAt time.Time) error {
	if m == nil {
		return nil
	}
	cacheSetSessionStatus(ctx, m.sessionStates, sessionID, status, expireAt)
	return nil
}

// WarmUserTokenVersion 将用户版本号写入缓存。
func (m *AuthStateManager) WarmUserTokenVersion(ctx context.Context, userID string, version int64) error {
	if m == nil {
		return nil
	}
	return m.userVersions.SetUserTokenVersion(ctx, userID, version)
}

func (m *AuthStateManager) currentTokenVersion(ctx context.Context, userID string) (int64, error) {
	if version, ok, err := m.userVersions.GetUserTokenVersion(ctx, userID); err == nil && ok {
		return version, nil
	}
	version, err := m.users.GetTokenVersion(ctx, userID)
	if err != nil {
		return 0, err
	}
	_ = m.userVersions.SetUserTokenVersion(ctx, userID, version)
	return version, nil
}

func (m *AuthStateManager) currentSessionStatus(ctx context.Context, sessionID string) (entity.AuthSessionStatus, error) {
	if status, ok, err := m.sessionStates.GetSessionStatus(ctx, sessionID); err == nil && ok {
		return status, nil
	}
	if m.sessions == nil {
		return entity.AuthSessionStatusActive, nil
	}
	session, err := m.sessions.GetByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return entity.AuthSessionStatusRevoked, nil
		}
		return "", err
	}
	status := session.Status
	if !session.IsUsable(time.Now()) {
		status = entity.AuthSessionStatusExpired
	}
	cacheSetSessionStatus(ctx, m.sessionStates, sessionID, status, session.ExpiresAt)
	return status, nil
}
