package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/repository"
	appcrypto "github.com/mikasa/mcp-manager/pkg/crypto"
	"github.com/mikasa/mcp-manager/pkg/response"
)

// AuthService 定义认证业务接口。
type AuthService interface {
	Login(ctx context.Context, username, password, ip, userAgent string) (*appcrypto.TokenPair, *entity.User, error)
	Logout(ctx context.Context, accessToken, refreshToken, userID, username, ip, userAgent string) error
	Refresh(ctx context.Context, refreshToken string) (*appcrypto.TokenPair, error)
	ChangePassword(ctx context.Context, userID, oldPassword, newPassword, operator, ip, userAgent string) error
}

type authService struct {
	users repository.UserRepository
	jwt   *appcrypto.JWTService
	audit AuditSink
}

// NewAuthService 创建认证服务。
func NewAuthService(users repository.UserRepository, jwtSvc *appcrypto.JWTService, audit AuditSink) AuthService {
	if audit == nil {
		audit = NoopAuditSink{}
	}
	return &authService{users: users, jwt: jwtSvc, audit: audit}
}

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
	pair, err := s.jwt.GenerateTokenPair(user.ID, user.Username, string(user.Role))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	_ = s.users.UpdateLastLogin(ctx, user.ID, now)
	_ = s.audit.Record(ctx, AuditEntry{
		UserID:       user.ID,
		Username:     user.Username,
		Action:       "login",
		ResourceType: "auth",
		Detail:       map[string]any{"username": user.Username},
		IPAddress:    ip,
		UserAgent:    userAgent,
	})
	user.LastLoginAt = &now
	return pair, user, nil
}

func (s *authService) Logout(ctx context.Context, accessToken, refreshToken, userID, username, ip, userAgent string) error {
	if accessToken != "" {
		if claims, err := s.jwt.ParseToken(accessToken, appcrypto.TokenTypeAccess); err == nil {
			s.jwt.Blacklist(accessToken, claims.ExpiresAt.Time)
		}
	}
	if refreshToken != "" {
		if claims, err := s.jwt.ParseToken(refreshToken, appcrypto.TokenTypeRefresh); err == nil {
			s.jwt.Blacklist(refreshToken, claims.ExpiresAt.Time)
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

func (s *authService) Refresh(ctx context.Context, refreshToken string) (*appcrypto.TokenPair, error) {
	pair, _, err := s.jwt.Refresh(refreshToken)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, response.NewBizError(http.StatusUnauthorized, response.CodeTokenExpired, "refresh token 已过期", err)
		}
		return nil, response.NewBizError(http.StatusUnauthorized, response.CodeUnauthorized, "refresh token 无效", err)
	}
	return pair, nil
}

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
	if err := s.users.UpdatePassword(ctx, userID, hashed); err != nil {
		return err
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
