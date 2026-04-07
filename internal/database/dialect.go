package database

import "gorm.io/gorm"

func isPostgres(db *gorm.DB) bool {
	return db != nil && db.Dialector.Name() == "postgres"
}
