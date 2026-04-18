package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

const refreshTokenEntropyBytes = 32

// GenerateRefreshToken 生成高熵 refresh token，并返回其哈希值。
func GenerateRefreshToken() (string, string, error) {
	buf := make([]byte, refreshTokenEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plain := "rt_" + base64.RawURLEncoding.EncodeToString(buf)
	return plain, HashRefreshToken(plain), nil
}

// HashRefreshToken 计算 refresh token 的 SHA-256 哈希值。
func HashRefreshToken(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
