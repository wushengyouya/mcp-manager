package database

import (
	"github.com/mikasa/mcp-manager/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func initPostgres(cfg config.DatabaseConfig) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(cfg.DSN), gormConfig())
}
