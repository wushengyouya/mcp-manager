package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var global *gorm.DB

// Init 初始化数据库连接
func Init(cfg config.DatabaseConfig) (*gorm.DB, error) {
	if cfg.Driver != "sqlite" {
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}

	if cfg.DSN != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(cfg.DSN), 0o755); err != nil {
			return nil, err
		}
	}

	dsn := cfg.DSN
	if dsn != ":memory:" {
		dsn = fmt.Sprintf("%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)", cfg.DSN)
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   newGormLogger(),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	global = db
	return db, nil
}

// newGormLogger 创建 GORM 使用的日志器
func newGormLogger() gormlogger.Interface {
	return gormlogger.New(log.New(os.Stdout, "", log.LstdFlags), gormlogger.Config{
		SlowThreshold:             time.Second,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})
}

// DB 返回全局数据库实例
func DB() *gorm.DB {
	return global
}

// Close 关闭数据库连接
func Close() error {
	if global == nil {
		return nil
	}
	sqlDB, err := global.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Health 检查数据库健康状态
func Health(ctx context.Context) error {
	if global == nil {
		return fmt.Errorf("database not initialized")
	}
	sqlDB, err := global.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Transaction 提供事务辅助
func Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	if global == nil {
		return fmt.Errorf("database not initialized")
	}
	return global.WithContext(ctx).Transaction(fn)
}

// SQLDB 返回底层 sql.DB
func SQLDB(db *gorm.DB) (*sql.DB, error) {
	return db.DB()
}
