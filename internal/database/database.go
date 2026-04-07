package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mikasa/mcp-manager/internal/config"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var global *gorm.DB

// Init 初始化数据库连接
func Init(cfg config.DatabaseConfig) (*gorm.DB, error) {
	var (
		db  *gorm.DB
		err error
	)
	switch cfg.Driver {
	case "sqlite":
		db, err = initSQLite(cfg)
	case "postgres":
		db, err = initPostgres(cfg)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", cfg.Driver)
	}
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

func gormConfig() *gorm.Config {
	return &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   newGormLogger(),
	}
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
