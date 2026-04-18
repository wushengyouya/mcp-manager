package crypto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestJWTService_GenerateParseAccess 验证 access JWT 的签发与解析。
func TestJWTService_GenerateParseAccess(t *testing.T) {
	svc := NewJWTService("secret", "issuer", time.Hour, 2*time.Hour, NewTokenBlacklist())
	token, expireAt, err := svc.GenerateAccessToken("u1", "sess-1", "root", "admin", 3)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.WithinDuration(t, time.Now().Add(time.Hour), expireAt, 2*time.Second)

	claims, err := svc.ParseAccessToken(t.Context(), token)
	require.NoError(t, err)
	require.Equal(t, "u1", claims.UserID)
	require.Equal(t, "sess-1", claims.SessionID)
	require.Equal(t, "root", claims.Username)
	require.Equal(t, "admin", claims.Role)
	require.EqualValues(t, 3, claims.TokenVersion)
	require.Equal(t, TokenTypeAccess, claims.Type)
}

// TestJWTService_GenerateTokenPairKeepsOpaqueRefresh 验证兼容 helper 返回 opaque refresh token。
func TestJWTService_GenerateTokenPairKeepsOpaqueRefresh(t *testing.T) {
	svc := NewJWTService("secret", "issuer", time.Hour, 2*time.Hour, NewTokenBlacklist())
	pair, err := svc.GenerateTokenPair("u1", "root", "admin")
	require.NoError(t, err)
	require.NotEmpty(t, pair.AccessToken)
	require.NotEmpty(t, pair.RefreshToken)

	_, err = svc.ParseAccessToken(t.Context(), pair.RefreshToken)
	require.Error(t, err)
}

// TestHashPassword_CheckPassword 验证密码哈希和比对逻辑
func TestHashPassword_CheckPassword(t *testing.T) {
	hashed, err := HashPassword("admin123")
	require.NoError(t, err)
	require.NoError(t, CheckPassword("admin123", hashed))
	require.Error(t, CheckPassword("bad", hashed))
}
