package crypto

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenType 定义令牌类型。
type TokenType string

const (
	// TokenTypeAccess 表示访问令牌。
	TokenTypeAccess TokenType = "access"
	// TokenTypeRefresh 表示刷新令牌。
	TokenTypeRefresh TokenType = "refresh"
)

var (
	// ErrInvalidTokenType 表示令牌类型非法。
	ErrInvalidTokenType = errors.New("invalid token type")
)

// Claims 定义 JWT 声明。
type Claims struct {
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
	Type     TokenType `json:"type"`
	jwt.RegisteredClaims
}

// TokenPair 定义访问令牌对。
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// JWTService 提供 JWT 能力。
type JWTService struct {
	secret     []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
	blacklist  *TokenBlacklist
}

// NewJWTService 创建 JWT 服务。
func NewJWTService(secret, issuer string, accessTTL, refreshTTL time.Duration, blacklist *TokenBlacklist) *JWTService {
	if blacklist == nil {
		blacklist = NewTokenBlacklist()
	}
	return &JWTService{
		secret:     []byte(secret),
		issuer:     issuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		blacklist:  blacklist,
	}
}

// GenerateTokenPair 生成令牌对。
func (s *JWTService) GenerateTokenPair(userID, username, role string) (*TokenPair, error) {
	accessToken, _, err := s.generateToken(userID, username, role, TokenTypeAccess, s.accessTTL)
	if err != nil {
		return nil, err
	}
	refreshToken, _, err := s.generateToken(userID, username, role, TokenTypeRefresh, s.refreshTTL)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: int64(s.accessTTL.Seconds())}, nil
}

func (s *JWTService) generateToken(userID, username, role string, tokenType TokenType, ttl time.Duration) (string, time.Time, error) {
	now := time.Now()
	expireAt := now.Add(ttl)
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		Type:     tokenType,
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

// ParseToken 解析令牌。
func (s *JWTService) ParseToken(tokenStr string, expectType TokenType) (*Claims, error) {
	if s.blacklist.Contains(tokenStr) {
		return nil, jwt.ErrTokenInvalidClaims
	}
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (any, error) {
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenMalformed
	}
	if claims.Type != expectType {
		return nil, ErrInvalidTokenType
	}
	return claims, nil
}

// Refresh 刷新令牌。
func (s *JWTService) Refresh(refreshToken string) (*TokenPair, *Claims, error) {
	claims, err := s.ParseToken(refreshToken, TokenTypeRefresh)
	if err != nil {
		return nil, nil, err
	}
	s.blacklist.Add(refreshToken, claims.ExpiresAt.Time)
	pair, err := s.GenerateTokenPair(claims.UserID, claims.Username, claims.Role)
	if err != nil {
		return nil, nil, err
	}
	return pair, claims, nil
}

// Blacklist 将令牌加入黑名单。
func (s *JWTService) Blacklist(token string, expireAt time.Time) {
	s.blacklist.Add(token, expireAt)
}

// BlacklistStore 返回黑名单实例。
func (s *JWTService) BlacklistStore() *TokenBlacklist {
	return s.blacklist
}
