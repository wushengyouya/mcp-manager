package repository

import (
	"context"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"gorm.io/gorm"
)

// AuthSessionRepository 定义认证会话仓储接口。
type AuthSessionRepository interface {
	Create(ctx context.Context, session *entity.AuthSession) error
	GetByID(ctx context.Context, id string) (*entity.AuthSession, error)
	GetByRefreshHash(ctx context.Context, hash string) (*entity.AuthSession, error)
	Update(ctx context.Context, session *entity.AuthSession) error
	Rotate(ctx context.Context, currentID, currentHash string, replacement *entity.AuthSession, usedAt time.Time) (bool, error)
	MarkReuseDetected(ctx context.Context, id string, detectedAt time.Time) (bool, error)
	RevokeByID(ctx context.Context, id, reason string, revokedAt time.Time) error
	RevokeByUserID(ctx context.Context, userID, reason string, revokedAt time.Time) (int64, error)
	ListActiveByUserID(ctx context.Context, userID string) ([]entity.AuthSession, error)
}

type authSessionRepository struct {
	db *gorm.DB
}

// NewAuthSessionRepository 创建认证会话仓储。
func NewAuthSessionRepository(db *gorm.DB) AuthSessionRepository {
	return &authSessionRepository{db: db}
}

func (r *authSessionRepository) Create(ctx context.Context, session *entity.AuthSession) error {
	return normalizeErr(r.db.WithContext(ctx).Create(session).Error)
}

func (r *authSessionRepository) GetByID(ctx context.Context, id string) (*entity.AuthSession, error) {
	var session entity.AuthSession
	if err := r.db.WithContext(ctx).First(&session, "id = ?", id).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &session, nil
}

func (r *authSessionRepository) GetByRefreshHash(ctx context.Context, hash string) (*entity.AuthSession, error) {
	var session entity.AuthSession
	if err := r.db.WithContext(ctx).First(&session, "refresh_token_hash = ?", hash).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return &session, nil
}

func (r *authSessionRepository) Update(ctx context.Context, session *entity.AuthSession) error {
	return normalizeErr(r.db.WithContext(ctx).Save(session).Error)
}

func (r *authSessionRepository) Rotate(ctx context.Context, currentID, currentHash string, replacement *entity.AuthSession, usedAt time.Time) (bool, error) {
	rotated := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&entity.AuthSession{}).
			Where("id = ? AND refresh_token_hash = ? AND status = ?", currentID, currentHash, entity.AuthSessionStatusActive).
			Updates(map[string]any{
				"status":       entity.AuthSessionStatusRotated,
				"replaced_by":  replacement.ID,
				"last_used_at": usedAt,
			})
		if res.Error != nil {
			return normalizeErr(res.Error)
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if err := tx.Create(replacement).Error; err != nil {
			return normalizeErr(err)
		}
		rotated = true
		return nil
	})
	return rotated, err
}

func (r *authSessionRepository) MarkReuseDetected(ctx context.Context, id string, detectedAt time.Time) (bool, error) {
	res := r.db.WithContext(ctx).Model(&entity.AuthSession{}).
		Where("id = ? AND status = ?", id, entity.AuthSessionStatusRotated).
		Updates(map[string]any{
			"status":        entity.AuthSessionStatusRevoked,
			"revoked_at":    detectedAt,
			"revoke_reason": "refresh_reuse",
		})
	if res.Error != nil {
		return false, normalizeErr(res.Error)
	}
	return res.RowsAffected > 0, nil
}

func (r *authSessionRepository) RevokeByID(ctx context.Context, id, reason string, revokedAt time.Time) error {
	updates := map[string]any{
		"status":        entity.AuthSessionStatusRevoked,
		"revoked_at":    revokedAt,
		"revoke_reason": reason,
	}
	res := r.db.WithContext(ctx).Model(&entity.AuthSession{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return normalizeErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *authSessionRepository) RevokeByUserID(ctx context.Context, userID, reason string, revokedAt time.Time) (int64, error) {
	updates := map[string]any{
		"status":        entity.AuthSessionStatusRevoked,
		"revoked_at":    revokedAt,
		"revoke_reason": reason,
	}
	res := r.db.WithContext(ctx).Model(&entity.AuthSession{}).
		Where("user_id = ? AND status = ?", userID, entity.AuthSessionStatusActive).
		Updates(updates)
	if res.Error != nil {
		return 0, normalizeErr(res.Error)
	}
	return res.RowsAffected, nil
}

func (r *authSessionRepository) ListActiveByUserID(ctx context.Context, userID string) ([]entity.AuthSession, error) {
	var sessions []entity.AuthSession
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, entity.AuthSessionStatusActive).
		Order("created_at desc").
		Find(&sessions).Error; err != nil {
		return nil, normalizeErr(err)
	}
	return sessions, nil
}
