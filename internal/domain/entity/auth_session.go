package entity

import "time"

// AuthSessionStatus 定义认证会话状态。
type AuthSessionStatus string

const (
	// AuthSessionStatusActive 表示会话可继续刷新。
	AuthSessionStatusActive AuthSessionStatus = "active"
	// AuthSessionStatusRotated 表示会话已被新 refresh token 替换。
	AuthSessionStatusRotated AuthSessionStatus = "rotated"
	// AuthSessionStatusRevoked 表示会话已被主动撤销。
	AuthSessionStatusRevoked AuthSessionStatus = "revoked"
	// AuthSessionStatusExpired 表示会话已过期。
	AuthSessionStatusExpired AuthSessionStatus = "expired"
)

// AuthSession 定义 refresh token 会话。
type AuthSession struct {
	Base
	UserID           string            `gorm:"type:varchar(36);index;not null" json:"user_id"`
	RefreshTokenHash string            `gorm:"type:varchar(64);uniqueIndex;not null" json:"-"`
	Status           AuthSessionStatus `gorm:"type:varchar(20);index;not null" json:"status"`
	TokenVersion     int64             `gorm:"not null;default:1" json:"token_version"`
	FamilyID         string            `gorm:"type:varchar(36);index;not null" json:"family_id"`
	ReplacedBy       *string           `gorm:"type:varchar(36)" json:"replaced_by,omitempty"`
	ClientIP         string            `gorm:"type:varchar(64)" json:"client_ip,omitempty"`
	UserAgent        string            `gorm:"type:text" json:"user_agent,omitempty"`
	LastUsedAt       *time.Time        `json:"last_used_at,omitempty"`
	ExpiresAt        time.Time         `gorm:"index;not null" json:"expires_at"`
	IdleExpiresAt    *time.Time        `gorm:"index" json:"idle_expires_at,omitempty"`
	RevokedAt        *time.Time        `json:"revoked_at,omitempty"`
	RevokeReason     string            `gorm:"type:varchar(64)" json:"revoke_reason,omitempty"`
}

// IsUsable 判断会话是否仍可用于 refresh。
func (s AuthSession) IsUsable(now time.Time) bool {
	if s.Status != AuthSessionStatusActive {
		return false
	}
	if !s.ExpiresAt.After(now) {
		return false
	}
	if s.IdleExpiresAt != nil && !s.IdleExpiresAt.After(now) {
		return false
	}
	return true
}
