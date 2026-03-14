package database

import (
	"context"
	"errors"
	"testing"

	"github.com/mikasa/mcp-manager/internal/config"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var errRollback = errors.New("force rollback")

func sqliteCfg() config.DatabaseConfig {
	return config.DatabaseConfig{
		Driver:       "sqlite",
		DSN:          ":memory:",
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	}
}

// TestInit_SQLite 验证使用 :memory: 初始化后 db 不为 nil。
func TestInit_SQLite(t *testing.T) {
	db, err := Init(sqliteCfg())
	require.NoError(t, err)
	require.NotNil(t, db)
	t.Cleanup(func() { _ = Close() })
}

// TestInit_UnsupportedDriver 验证不支持的驱动返回错误。
func TestInit_UnsupportedDriver(t *testing.T) {
	cfg := sqliteCfg()
	cfg.Driver = "mysql"
	db, err := Init(cfg)
	require.Error(t, err)
	require.Nil(t, db)
	require.Contains(t, err.Error(), "unsupported database driver")
}

// TestHealth 验证初始化后 Health 返回 nil。
func TestHealth(t *testing.T) {
	_, err := Init(sqliteCfg())
	require.NoError(t, err)
	t.Cleanup(func() { _ = Close() })

	err = Health(context.Background())
	require.NoError(t, err)
}

// TestTransaction_Commit 验证事务正常提交。
func TestTransaction_Commit(t *testing.T) {
	db, err := Init(sqliteCfg())
	require.NoError(t, err)
	t.Cleanup(func() { _ = Close() })
	require.NoError(t, Migrate(db))

	ctx := context.Background()
	err = Transaction(ctx, func(tx *gorm.DB) error {
		return tx.Create(&entity.User{
			Username: "txuser",
			Password: "hashed",
			Email:    "tx@test.com",
			Role:     entity.RoleReadonly,
			IsActive: true,
		}).Error
	})
	require.NoError(t, err)

	var count int64
	db.Model(&entity.User{}).Where("username = ?", "txuser").Count(&count)
	require.Equal(t, int64(1), count)
}

// TestTransaction_Rollback 验证事务返回错误时回滚。
func TestTransaction_Rollback(t *testing.T) {
	db, err := Init(sqliteCfg())
	require.NoError(t, err)
	t.Cleanup(func() { _ = Close() })
	require.NoError(t, Migrate(db))

	ctx := context.Background()
	err = Transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&entity.User{
			Username: "rollback_user",
			Password: "hashed",
			Email:    "rb@test.com",
			Role:     entity.RoleReadonly,
			IsActive: true,
		}).Error; err != nil {
			return err
		}
		return errRollback
	})
	require.Error(t, err)

	var count int64
	db.Model(&entity.User{}).Where("username = ?", "rollback_user").Count(&count)
	require.Equal(t, int64(0), count)
}

// TestMigrate 验证迁移后 5 张表全部存在。
func TestMigrate(t *testing.T) {
	db, err := Init(sqliteCfg())
	require.NoError(t, err)
	t.Cleanup(func() { _ = Close() })

	require.NoError(t, Migrate(db))

	require.True(t, db.Migrator().HasTable(&entity.User{}), "users table")
	require.True(t, db.Migrator().HasTable(&entity.MCPService{}), "mcp_services table")
	require.True(t, db.Migrator().HasTable(&entity.Tool{}), "tools table")
	require.True(t, db.Migrator().HasTable(&entity.RequestHistory{}), "request_histories table")
	require.True(t, db.Migrator().HasTable(&entity.AuditLog{}), "audit_logs table")
}

// TestSQLDB 验证 SQLDB 返回有效的 sql.DB。
func TestSQLDB(t *testing.T) {
	db, err := Init(sqliteCfg())
	require.NoError(t, err)
	t.Cleanup(func() { _ = Close() })

	sqlDB, err := SQLDB(db)
	require.NoError(t, err)
	require.NotNil(t, sqlDB)
	require.NoError(t, sqlDB.Ping())
}
