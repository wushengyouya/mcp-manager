package database

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mikasa/mcp-manager/internal/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func initSQLite(cfg config.DatabaseConfig) (*gorm.DB, error) {
	if cfg.DSN != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(cfg.DSN), 0o755); err != nil {
			return nil, err
		}
	}

	dsn := cfg.DSN
	if dsn != ":memory:" {
		dsn = fmt.Sprintf("%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)", cfg.DSN)
	}

	return gorm.Open(sqlite.Open(dsn), gormConfig())
}
