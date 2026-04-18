package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// AuthService 定义认证业务接口
type AuthService interface {
	Login(ctx context.Context, username, password, ip, userAgent string) (*appcrypto.TokenPair, *entity.User, error)
	Logout(ctx context.Context, accessToken, refreshToken, userID, username, ip, userAgent string) error
	Refresh(ctx context.Context, refreshToken string) (*appcrypto.TokenPair, error)
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword, operator, ip, userAgent string) error
}

// AuthServiceOption 定义认证服务选项。
type AuthServiceOption func(*authService)

// WithAuthStateManager 注入认证状态管理器。
func WithAuthStateManager(manager *AuthStateManager) AuthServiceOption {
	return func(s *authService) {
		s.authState = manager
	}
}

// authService 实现认证业务接口。
type authService struct {
	users     repository.UserRepository
	sessions  repository.AuthSessionRepository
	jwt       *appcrypto.JWTService
	audit     AuditSink
	authState *AuthStateManager
}

// NewAuthService 创建认证服务
func NewAuthService(users repository.UserRepository, sessions repository.AuthSessionRepository, jwtSvc *appcrypto.JWTService, audit AuditSink, opts ...AuthServiceOption) AuthService {
	if audit == nil {
		audit = NoopAuditSink{}
	}
	svc := &authService{users: users, sessions: sessions, jwt: jwtSvc, audit: audit}
	for _, apply := range opts {
		if apply != nil {
			apply(svc)
		}
	}
	return svc
}

// Login 校验用户名密码并签发令牌
func (s *authService) Login(ctx context.Context, username, password, ip, userAgent string) (*appcrypto.TokenPair, *entity.User, error) {
	user, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil, response.NewBizError(http.StatusUnauthorized, response.CodeUnauthorized, "用户名或密码错误", err)
		}
		return nil, nil, err
	}
	if !user.IsActive {
		return nil, nil, response.NewBizError(http.StatusForbidden, response.CodeForbidden, "用户已禁用", nil)
	}
	if err := appcrypto.CheckPassword(password, user.Password); err != nil {
		return nil, nil, response.NewBizError(http.StatusUnauthorized, response.CodeUnauthorized, "用户名或密码错误", err)
	}

	sessionID := uuid.NewString()
	accessToken, _, err := s.jwt.GenerateAccessToken(user.ID, sessionID, user.Username, string(user.Role), user.TokenVersion)
	if err != nil {
		return nil, nil, err
	}
	refreshToken, refreshHash, err := appcrypto.GenerateRefreshToken()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	sessionExpireAt := now.Add(s.jwt.RefreshTTL())
	session := &entity.AuthSession{
		Base:             entity.Base{ID: sessionID},
		UserID:           user.ID,
		RefreshTokenHash: refreshHash,
		Status:           entity.AuthSessionStatusActive,
		TokenVersion:     user.TokenVersion,
		FamilyID:         sessionID,
		ClientIP:         ip,
		UserAgent:        userAgent,
		LastUsedAt:       &now,
		ExpiresAt:        sessionExpireAt,
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return nil, nil, err
	}
	if s.authState != nil {
		_ = s.authState.MarkSessionStatus(ctx, session.ID, entity.AuthSessionStatusActive, session.ExpiresAt)
		_ = s.authState.WarmUserTokenVersion(ctx, user.ID, user.TokenVersion)
	}
	_ = s.users.UpdateLastLogin(ctx, user.ID, now)
	_ = s.audit.Record(ctx, AuditEntry{
		UserID:       user.ID,
		Username:     user.Username,
		Action:       "login",
		ResourceType: "auth",
		Detail:       map[string]any{"username": user.Username, "session_id": session.ID},
		IPAddress:    ip,
		UserAgent:    userAgent,
	})
	user.LastLoginAt = &now
	return &appcrypto.TokenPair{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: int64(s.jwt.AccessTTL().Seconds())}, user, nil
}

// Logout 撤销当前 access token 所属会话并写入 access 黑名单。
func (s *authService) Logout(ctx context.Context, accessToken, refreshToken, userID, username, ip, userAgent string) error {
	if accessToken != "" {
		if claims, err := s.jwt.ParseAccessToken(ctx, accessToken); err == nil {
			s.jwt.Blacklist(accessToken, claims.ExpiresAt.Time)
			if s.authState != nil {
				if err := s.authState.RevokeSession(ctx, claims.SessionID, "logout"); err != nil {
					return err
				}
			}
			if userID == "" {
				userID = claims.UserID
			}
			if username == "" {
				username = claims.Username
			}
		}
	}
	if refreshToken != "" && s.sessions != nil {
		if session, err := s.sessions.GetByRefreshHash(ctx, appcrypto.HashRefreshToken(refreshToken)); err == nil && s.authState != nil {
			if err := s.authState.RevokeSession(ctx, session.ID, "logout"); err != nil {
				return err
			}
		}
	}
	return s.audit.Record(ctx, AuditEntry{
		UserID:       userID,
		Username:     username,
		Action:       "logout",
		ResourceType: "auth",
		Detail:       map[string]any{},
		IPAddress:    ip,
		UserAgent:    userAgent,
	})
}

// Refresh 使用 opaque refresh token 轮换新的令牌对。
func (s *authService) Refresh(ctx context.Context, refreshToken string) (*appcrypto.TokenPair, error) {
	if refreshToken == "" {
		return nil, response.NewBizError(http.StatusUnauthorized, response.CodeUnauthorized, "refresh token 无效", nil)
	}
	refreshHash := appcrypto.HashRefreshToken(refreshToken)
	session, err := s.sessions.GetByRefreshHash(ctx, refreshHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, response.NewBizError(http.StatusUnauthorized, response.CodeUnauthorized, "refresh token 无效", err)
		}
		return nil, err
	}
	now := time.Now()
	if session.Status == entity.AuthSessionStatusRotated {
		marked, err := s.sessions.MarkReuseDetected(ctx, session.ID, now)
		if err != nil {
			return nil, err
		}
		if marked && s.authState != nil {
			if _, err := s.authState.RevokeAllForUser(ctx, session.UserID, "refresh_reuse"); err != nil {
				return nil, err
			}
		}
		return nil, response.NewBizError(http.StatusUnauthorized, response.CodeUnauthorized, "refresh token 无效", nil)
	}
	if !session.IsUsable(now) {
		return nil, response.NewBizError(http.StatusUnauthorized, response.CodeTokenExpired, "refresh token 已过期", nil)
	}

	user, err := s.users.GetByID(ctx, session.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, response.NewBizError(http.StatusUnauthorized, response.CodeUnauthorized, "refresh token 无效", err)
		}
		return nil, err
	}
	if !user.IsActive || session.TokenVersion != user.TokenVersion {
		return nil, response.NewBizError(http.StatusUnauthorized, response.CodeUnauthorized, "refresh token 无效", nil)
	}

	newSessionID := uuid.NewString()
	accessToken, _, err := s.jwt.GenerateAccessToken(user.ID, newSessionID, user.Username, string(user.Role), user.TokenVersion)
	if err != nil {
		return nil, err
	}
	newRefreshToken, newRefreshHash, err := appcrypto.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	newExpireAt := now.Add(s.jwt.RefreshTTL())
	newSession := &entity.AuthSession{
		Base:             entity.Base{ID: newSessionID},
		UserID:           user.ID,
		RefreshTokenHash: newRefreshHash,
		Status:           entity.AuthSessionStatusActive,
		TokenVersion:     user.TokenVersion,
		FamilyID:         session.FamilyID,
		ClientIP:         session.ClientIP,
		UserAgent:        session.UserAgent,
		LastUsedAt:       &now,
		ExpiresAt:        newExpireAt,
	}
	rotated, err := s.sessions.Rotate(ctx, session.ID, refreshHash, newSession, now)
	if err != nil {
		return nil, err
	}
	if !rotated {
		latest, getErr := s.sessions.GetByID(ctx, session.ID)
		if getErr == nil && latest.Status == entity.AuthSessionStatusRotated {
			marked, err := s.sessions.MarkReuseDetected(ctx, session.ID, now)
			if err != nil {
				return nil, err
			}
			if marked && s.authState != nil {
				if _, err := s.authState.RevokeAllForUser(ctx, session.UserID, "refresh_reuse"); err != nil {
					return nil, err
				}
			}
		}
		return nil, response.NewBizError(http.StatusUnauthorized, response.CodeUnauthorized, "refresh token 无效", nil)
	}
	if s.authState != nil {
		_ = s.authState.MarkSessionStatus(ctx, session.ID, entity.AuthSessionStatusRotated, session.ExpiresAt)
		_ = s.authState.MarkSessionStatus(ctx, newSession.ID, entity.AuthSessionStatusActive, newSession.ExpiresAt)
		_ = s.authState.WarmUserTokenVersion(ctx, user.ID, user.TokenVersion)
	}
	_ = s.audit.Record(ctx, AuditEntry{
		UserID:       user.ID,
		Username:     user.Username,
		Action:       "refresh",
		ResourceType: "auth",
		Detail:       map[string]any{"session_id": newSession.ID, "rotated_from": session.ID},
	})
	return &appcrypto.TokenPair{AccessToken: accessToken, RefreshToken: newRefreshToken, ExpiresIn: int64(s.jwt.AccessTTL().Seconds())}, nil
}

// ChangePassword 校验旧密码并更新新密码，同时撤销全部历史会话。
func (s *authService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword, operator, ip, userAgent string) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := appcrypto.CheckPassword(oldPassword, user.Password); err != nil {
		return response.NewBizError(http.StatusBadRequest, response.CodeInvalidArgument, "旧密码错误", err)
	}
	hashed, err := appcrypto.HashPassword(newPassword)
	if err != nil {
		return response.NewBizError(http.StatusBadRequest, response.CodeInvalidArgument, "新密码不合法", err)
	}
	newVersion, err := s.users.UpdatePasswordAndBumpTokenVersion(ctx, userID, hashed)
	if err != nil {
		return err
	}
	if s.authState != nil {
		if err := s.authState.WarmUserTokenVersion(ctx, userID, newVersion); err != nil {
			return err
		}
		if err := s.authState.RevokeSessionsForUser(ctx, userID, "password_changed"); err != nil {
			return err
		}
	}
	return s.audit.Record(ctx, AuditEntry{
		UserID:       userID,
		Username:     operator,
		Action:       "change_password",
		ResourceType: "user",
		ResourceID:   userID,
		Detail:       map[string]any{},
		IPAddress:    ip,
		UserAgent:    userAgent,
	})
}
