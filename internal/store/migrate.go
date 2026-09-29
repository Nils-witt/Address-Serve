package store

import (
	"context"

	"gorm.io/gorm"
)

// Migrate creates the schema, or adds missing tables, columns and indexes to
// an existing one. It is safe to run on each startup.
func Migrate(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).AutoMigrate(&Street{}, &HouseNumber{})
}
