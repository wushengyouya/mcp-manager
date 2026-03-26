package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNormalizeErr(t *testing.T) {
	require.ErrorIs(t, normalizeErr(gorm.ErrRecordNotFound), ErrNotFound)
	err := errors.New("other")
	require.ErrorIs(t, normalizeErr(err), err)
}

func TestIsUniqueErr(t *testing.T) {
	require.False(t, isUniqueErr(nil))
	require.True(t, isUniqueErr(errors.New("UNIQUE constraint failed: users.username")))
	require.False(t, isUniqueErr(errors.New("duplicate key")))
}

func TestExists(t *testing.T) {
	db := setupRepositoryTestDB(t)
	repo := NewUserRepository(db)
	user := seedUser(t, repo, "exists-user", "exists@example.com", entity.RoleAdmin, true)

	ok, err := exists(context.Background(), db, &entity.User{}, "id = ?", user.ID)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = exists(context.Background(), db, &entity.User{}, "id = ?", "missing")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestNormalizePage(t *testing.T) {
	page, size := normalizePage(0, 0)
	require.Equal(t, 1, page)
	require.Equal(t, 10, size)

	page, size = normalizePage(2, 20)
	require.Equal(t, 2, page)
	require.Equal(t, 20, size)

	page, size = normalizePage(1, 101)
	require.Equal(t, 1, page)
	require.Equal(t, 10, size)
}

func TestContainsAndStringIndex(t *testing.T) {
	require.True(t, contains("abcdef", "abc"))
	require.True(t, contains("abcdef", ""))
	require.False(t, contains("abcdef", "gh"))
	require.Equal(t, 2, stringIndex("abcdef", "cd"))
	require.Equal(t, -1, stringIndex("abcdef", "gh"))
}
