package crypto

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenType 定义令牌类型
type TokenType string

const (
	// TokenTypeAccess 表示访问令牌
	TokenTypeAccess TokenType = "access"
)

var (
	// ErrInvalidTokenType 表示令牌类型非法
	ErrInvalidTokenType = errors.New("invalid token type")
	// ErrTokenVersionMismatch 表示 access token 版本已过期。
	ErrTokenVersionMismatch = errors.New("token version mismatch")
	// ErrSessionRevoked 表示 access token 所属会话已失效。
	ErrSessionRevoked = errors.New("session revoked")
	// ErrRefreshTokenJWTUnsupported 表示不再支持 JWT refresh token。
	ErrRefreshTokenJWTUnsupported = errors.New("refresh jwt is no longer supported")
)

// AccessClaims 定义 access token 声明。
type AccessClaims struct {
	UserID       string    `json:"sub"`
	SessionID    string    `json:"sid"`
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	TokenVersion int64     `json:"ver"`
	Type         TokenType `json:"typ"`
	JTI          string    `json:"jti"`
	jwt.RegisteredClaims
}

// TokenPair 定义访问令牌对
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// AccessTokenValidator 定义 access token 状态校验器。
type AccessTokenValidator interface {
	ValidateAccessToken(ctx context.Context, claims *AccessClaims) error
}

// JWTService 提供 access JWT 能力。
type JWTService struct {
	secret          []byte
	issuer          string
	accessTTL       time.Duration
	refreshTTL      time.Duration
	blacklist       TokenBlacklistStore
	accessValidator AccessTokenValidator
}

// NewJWTService 创建 JWT 服务。
func NewJWTService(secret, issuer string, accessTTL, refreshTTL time.Duration, blacklist TokenBlacklistStore) *JWTService {
	if blacklist == nil {
		blacklist = NewInMemoryTokenBlacklistStore()
	}
	return &JWTService{
		secret:     []byte(secret),
		issuer:     issuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		blacklist:  blacklist,
	}
}

// SetAccessTokenValidator 设置 access token 状态校验器。
func (s *JWTService) SetAccessTokenValidator(validator AccessTokenValidator) {
	s.accessValidator = validator
}

// AccessTTL 返回 access token 过期时长。
func (s *JWTService) AccessTTL() time.Duration {
	return s.accessTTL
}

// RefreshTTL 返回 refresh token/会话过期时长。
func (s *JWTService) RefreshTTL() time.Duration {
	return s.refreshTTL
}

// GenerateAccessToken 生成 access token。
func (s *JWTService) GenerateAccessToken(userID, sessionID, username, role string, tokenVersion int64) (string, time.Time, error) {
	now := time.Now()
	expireAt := now.Add(s.accessTTL)
	claims := AccessClaims{
		UserID:       userID,
		SessionID:    sessionID,
		Username:     username,
		Role:         role,
		TokenVersion: tokenVersion,
		Type:         TokenTypeAccess,
		JTI:          uuid.NewString(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expireAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	return signed, expireAt, err
}

// GenerateTokenPair 为测试与兼容调用方生成 access + opaque refresh token。
func (s *JWTService) GenerateTokenPair(userID, username, role string) (*TokenPair, error) {
	accessToken, _, err := s.GenerateAccessToken(userID, uuid.NewString(), username, role, 1)
	if err != nil {
		return nil, err
	}
	refreshToken, _, err := GenerateRefreshToken()
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: int64(s.accessTTL.Seconds())}, nil
}

// ParseAccessToken 解析并校验 access token。
func (s *JWTService) ParseAccessToken(ctx context.Context, tokenStr string) (*AccessClaims, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.blacklist.Contains(tokenStr) {
		return nil, jwt.ErrTokenInvalidClaims
	}
	token, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, jwt.ErrTokenUnverifiable
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*AccessClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenMalformed
	}
	if claims.Type != TokenTypeAccess {
		return nil, ErrInvalidTokenType
	}
	if s.accessValidator != nil {
		if err := s.accessValidator.ValidateAccessToken(ctx, claims); err != nil {
			return nil, err
		}
	}
	return claims, nil
}

// ParseToken 为旧调用方保留 access token 解析兼容入口。
func (s *JWTService) ParseToken(tokenStr string, expectType TokenType) (*AccessClaims, error) {
	if expectType != TokenTypeAccess {
		return nil, ErrInvalidTokenType
	}
	return s.ParseAccessToken(context.Background(), tokenStr)
}

// Refresh 对旧 JWT refresh token 直接返回不支持。
func (s *JWTService) Refresh(string) (*TokenPair, *AccessClaims, error) {
	return nil, nil, ErrRefreshTokenJWTUnsupported
}

// Blacklist 将 access token 加入黑名单。
func (s *JWTService) Blacklist(token string, expireAt time.Time) {
	s.blacklist.Add(token, expireAt)
}

// BlacklistStore 返回黑名单实例。
func (s *JWTService) BlacklistStore() TokenBlacklistStore {
	return s.blacklist
}
