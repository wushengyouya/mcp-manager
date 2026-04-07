package database

import (
	"fmt"

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
				if err := tx.AutoMigrate(
					&entity.User{},
					&entity.MCPService{},
					&entity.Tool{},
					&entity.RequestHistory{},
					&entity.AuditLog{},
				); err != nil {
					return err
				}
				return applyDialectDDL(tx)
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
				return applyActiveUniqueIndexes(tx)
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

func applyDialectDDL(tx *gorm.DB) error {
	if err := applyJSONColumns(tx); err != nil {
		return err
	}
	return applyActiveUniqueIndexes(tx)
}

func applyJSONColumns(tx *gorm.DB) error {
	if !isPostgres(tx) {
		return nil
	}

	type columnSpec struct {
		table        string
		column       string
		defaultValue string
	}

	specs := []columnSpec{
		{table: "mcp_services", column: "args", defaultValue: "[]"},
		{table: "mcp_services", column: "env", defaultValue: "{}"},
		{table: "mcp_services", column: "custom_headers", defaultValue: "{}"},
		{table: "mcp_services", column: "tags", defaultValue: "[]"},
		{table: "tools", column: "input_schema", defaultValue: "{}"},
		{table: "request_histories", column: "request_body", defaultValue: "{}"},
		{table: "request_histories", column: "response_body", defaultValue: "{}"},
		{table: "audit_logs", column: "detail", defaultValue: "{}"},
	}

	for _, spec := range specs {
		stmt := fmt.Sprintf(
			`ALTER TABLE %s ALTER COLUMN %s TYPE JSONB USING COALESCE(NULLIF(%s::text, ''), '%s')::jsonb`,
			spec.table,
			spec.column,
			spec.column,
			spec.defaultValue,
		)
		if err := tx.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyActiveUniqueIndexes(tx *gorm.DB) error {
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
}
