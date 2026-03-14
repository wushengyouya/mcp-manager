package crypto

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHashPassword 测试多种长度密码的哈希生成。
func TestHashPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"min_length_6", "abcdef", false},
		{"medium_length_20", "abcdefghij1234567890", false},
		{"max_length_72", strings.Repeat("x", 72), false},
		{"too_short_5", "abcde", true},
		{"too_long_73", strings.Repeat("x", 73), true},
		{"empty_string", "", true},
		{"single_char", "a", true},
		{"unicode_6_bytes", "你好", false}, // 你好 = 6 bytes in UTF-8
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashed, err := HashPassword(tt.password)
			if tt.wantErr {
				require.Error(t, err)
				require.Empty(t, hashed)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, hashed)
				require.NoError(t, CheckPassword(tt.password, hashed))
			}
		})
	}
}

// TestHashPassword_DifferentHashes 测试相同密码每次生成不同的哈希值。
func TestHashPassword_DifferentHashes(t *testing.T) {
	pwd := "same_password"
	h1, err := HashPassword(pwd)
	require.NoError(t, err)
	h2, err := HashPassword(pwd)
	require.NoError(t, err)
	require.NotEqual(t, h1, h2, "bcrypt should produce different hashes for the same password")
}

// TestCheckPassword_WrongPassword 测试错误密码校验返回错误。
func TestCheckPassword_WrongPassword(t *testing.T) {
	hashed, err := HashPassword("correct")
	require.NoError(t, err)

	tests := []struct {
		name  string
		wrong string
	}{
		{"completely_different", "wrongpassword"},
		{"off_by_one_char", "correc"},
		{"extra_char", "correct!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Error(t, CheckPassword(tt.wrong, hashed))
		})
	}
}

// TestValidatePassword 测试密码长度校验的边界情况。
func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"exactly_6", "123456", false},
		{"exactly_72", strings.Repeat("a", 72), false},
		{"5_bytes", "12345", true},
		{"73_bytes", strings.Repeat("a", 73), true},
		{"empty", "", true},
		{"valid_utf8_multibyte", "密码测试ok", false}, // 12+2 = 14 bytes
		{"boundary_72_multibyte", strings.Repeat("啊", 24), false}, // 24*3 = 72 bytes
		{"boundary_73_multibyte", strings.Repeat("啊", 24) + "x", true}, // 73 bytes
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrPasswordLength)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// BenchmarkHashPassword 基准测试密码哈希性能。
func BenchmarkHashPassword(b *testing.B) {
	for b.Loop() {
		_, _ = HashPassword("benchmark1")
	}
}
