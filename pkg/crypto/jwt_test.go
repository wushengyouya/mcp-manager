package crypto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJWTService_GenerateParseRefresh(t *testing.T) {
	svc := NewJWTService("secret", "issuer", time.Hour, 2*time.Hour, NewTokenBlacklist())
	pair, err := svc.GenerateTokenPair("u1", "root", "admin")
	require.NoError(t, err)
	require.NotEmpty(t, pair.AccessToken)
	require.NotEmpty(t, pair.RefreshToken)

	claims, err := svc.ParseToken(pair.AccessToken, TokenTypeAccess)
	require.NoError(t, err)
	require.Equal(t, "u1", claims.UserID)
	require.Equal(t, "root", claims.Username)

	next, parsed, err := svc.Refresh(pair.RefreshToken)
	require.NoError(t, err)
	require.Equal(t, "u1", parsed.UserID)
	require.NotEmpty(t, next.AccessToken)
	_, err = svc.ParseToken(pair.RefreshToken, TokenTypeRefresh)
	require.Error(t, err)
}

func TestHashPassword_CheckPassword(t *testing.T) {
	hashed, err := HashPassword("admin123")
	require.NoError(t, err)
	require.NoError(t, CheckPassword("admin123", hashed))
	require.Error(t, CheckPassword("bad", hashed))
}
