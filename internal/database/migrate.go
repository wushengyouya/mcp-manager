package database

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"gorm.io/gorm"
)

// Migrate 执行数据库迁移
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
		{
			ID: "202603230001_active_unique_indexes",
			Migrate: func(tx *gorm.DB) error {
				statements := []string{
					`DROP INDEX IF EXISTS idx_service_tool`,
					`DROP INDEX IF EXISTS idx_service_tool_active`,
					`CREATE UNIQUE INDEX IF NOT EXISTS idx_service_tool_active ON tools(mcp_service_id, name) WHERE deleted_at IS NULL`,
					`DROP INDEX IF EXISTS idx_mcp_services_name`,
					`DROP INDEX IF EXISTS idx_mcp_services_name_active`,
					`CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_services_name_active ON mcp_services(name) WHERE deleted_at IS NULL`,
				}
				for _, stmt := range statements {
					if err := tx.Exec(stmt).Error; err != nil {
						return err
					}
				}
				return nil
			},
			Rollback: func(tx *gorm.DB) error {
				statements := []string{
					`DROP INDEX IF EXISTS idx_service_tool_active`,
					`CREATE UNIQUE INDEX IF NOT EXISTS idx_service_tool ON tools(name)`,
					`DROP INDEX IF EXISTS idx_mcp_services_name_active`,
					`CREATE UNIQUE INDEX IF NOT EXISTS idx_mcp_services_name ON mcp_services(name)`,
				}
				for _, stmt := range statements {
					if err := tx.Exec(stmt).Error; err != nil {
						return err
					}
				}
				return nil
			},
		},
	})
	return m.Migrate()
}
