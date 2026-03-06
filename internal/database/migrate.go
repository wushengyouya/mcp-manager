package database

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"gorm.io/gorm"
)

// Migrate 执行数据库迁移。
func Migrate(db *gorm.DB) error {
	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		{
			ID: "202603060001_init",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(
					&entity.User{},
					&entity.MCPService{},
					&entity.Tool{},
					&entity.RequestHistory{},
					&entity.AuditLog{},
				)
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable(
					&entity.AuditLog{},
					&entity.RequestHistory{},
					&entity.Tool{},
					&entity.MCPService{},
					&entity.User{},
				)
			},
		},
	})
	return m.Migrate()
}
