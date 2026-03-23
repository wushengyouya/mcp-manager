package crypto

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrPasswordLength 表示密码长度非法
	ErrPasswordLength = errors.New("password length must be 6-72 bytes")
)

const defaultCost = 10

// HashPassword 生成密码哈希
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	out, err := bcrypt.GenerateFromPassword([]byte(password), defaultCost)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CheckPassword 校验密码
func CheckPassword(password, hashed string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
}

// ValidatePassword 校验密码长度
func ValidatePassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrPasswordLength
	}
	n := len([]byte(password))
	if n < 6 || n > 72 {
		return ErrPasswordLength
	}
	return nil
}
