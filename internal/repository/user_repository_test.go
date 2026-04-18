package repository

import (
	"context"
	"testing"
	"time"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
)

func TestUserRepositoryCreateAndGetters(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewUserRepository(db)

	user := &entity.User{
		Username:     "alice",
		Password:     "hashed",
		Email:        "alice@example.com",
		Role:         entity.RoleAdmin,
		IsActive:     true,
		IsFirstLogin: true,
	}
	require.NoError(t, repo.Create(context.Background(), user))
	require.NotEmpty(t, user.ID)

	gotByID, err := repo.GetByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", gotByID.Username)

	gotByUsername, err := repo.GetByUsername(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, user.ID, gotByUsername.ID)

	gotByEmail, err := repo.GetByEmail(context.Background(), "alice@example.com")
	require.NoError(t, err)
	require.Equal(t, user.ID, gotByEmail.ID)
}

func TestUserRepositoryCreateUniqueConflict(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewUserRepository(db)
	seedUser(t, repo, "duplicate", "dup@example.com", entity.RoleAdmin, true)

	err := repo.Create(context.Background(), &entity.User{
		Username: "duplicate",
		Password: "hashed",
		Email:    "other@example.com",
		Role:     entity.RoleReadonly,
		IsActive: true,
	})
	require.ErrorIs(t, err, ErrAlreadyExists)
}

func TestUserRepositoryUpdateUniqueConflict(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewUserRepository(db)
	first := seedUser(t, repo, "first", "first@example.com", entity.RoleAdmin, true)
	second := seedUser(t, repo, "second", "second@example.com", entity.RoleReadonly, true)

	second.Email = first.Email
	err := repo.Update(context.Background(), second)
	require.ErrorIs(t, err, ErrAlreadyExists)
}

func TestUserRepositoryDeleteAndNotFound(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewUserRepository(db)
	user := seedUser(t, repo, "delete-me", "delete@example.com", entity.RoleReadonly, true)

	require.NoError(t, repo.Delete(context.Background(), user.ID))

	_, err := repo.GetByID(context.Background(), user.ID)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, repo.Delete(context.Background(), user.ID), ErrNotFound)
}

func TestUserRepositoryNotFoundLookups(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewUserRepository(db)

	_, err := repo.GetByID(context.Background(), "missing")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = repo.GetByUsername(context.Background(), "missing")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = repo.GetByEmail(context.Background(), "missing@example.com")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestUserRepositoryExistsAndList(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewUserRepository(db)
	seedUser(t, repo, "admin-a", "admin-a@example.com", entity.RoleAdmin, true)
	seedUser(t, repo, "operator-a", "operator-a@example.com", entity.RoleOperator, true)
	seedUser(t, repo, "readonly-a", "readonly-a@example.com", entity.RoleReadonly, false)

	ok, err := repo.ExistsByUsername(context.Background(), "admin-a")
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = repo.ExistsByEmail(context.Background(), "missing@example.com")
	require.NoError(t, err)
	require.False(t, ok)

	active := true
	items, total, err := repo.List(context.Background(), UserListFilter{
		Page:     1,
		PageSize: 1,
		Role:     string(entity.RoleOperator),
		Active:   &active,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	require.Equal(t, entity.RoleOperator, items[0].Role)
}

func TestUserRepositoryUpdateLastLoginPasswordAndFirstLogin(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewUserRepository(db)
	user := seedUser(t, repo, "state-user", "state@example.com", entity.RoleReadonly, true)
	now := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)

	require.NoError(t, repo.UpdateLastLogin(context.Background(), user.ID, now))
	require.NoError(t, repo.UpdatePassword(context.Background(), user.ID, "new-hash"))
	require.NoError(t, repo.SetFirstLoginFalse(context.Background(), user.ID))

	got, err := repo.GetByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastLoginAt)
	require.Equal(t, now, got.LastLoginAt.UTC().Truncate(time.Second))
	require.Equal(t, "new-hash", got.Password)
	require.False(t, got.IsFirstLogin)
}

func TestUserRepositoryUpdateAndBumpTokenVersion(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewUserRepository(db)
	user := seedUser(t, repo, "bump-user", "bump@example.com", entity.RoleReadonly, true)

	user.Email = "bump-new@example.com"
	user.Role = entity.RoleOperator
	user.IsActive = false
	version, err := repo.UpdateAndBumpTokenVersion(context.Background(), user)
	require.NoError(t, err)
	require.EqualValues(t, 2, version)

	got, err := repo.GetByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, "bump-new@example.com", got.Email)
	require.Equal(t, entity.RoleOperator, got.Role)
	require.False(t, got.IsActive)
	require.EqualValues(t, 2, got.TokenVersion)
}

func TestUserRepositoryUpdatePasswordAndBumpTokenVersion(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewUserRepository(db)
	user := seedUser(t, repo, "pwd-user", "pwd@example.com", entity.RoleReadonly, true)

	version, err := repo.UpdatePasswordAndBumpTokenVersion(context.Background(), user.ID, "next-hash")
	require.NoError(t, err)
	require.EqualValues(t, 2, version)

	got, err := repo.GetByID(context.Background(), user.ID)
	require.NoError(t, err)
	require.Equal(t, "next-hash", got.Password)
	require.False(t, got.IsFirstLogin)
	require.EqualValues(t, 2, got.TokenVersion)
}
