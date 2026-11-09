package store

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Migrate creates the schema, or adds missing tables, columns and indexes to
// an existing one. It is safe to run on each startup.
//
// It needs the cube and earthdistance extensions, which are trusted, so a
// database owner without superuser rights can create them.
func Migrate(ctx context.Context, db *gorm.DB) error {
	db = db.WithContext(ctx)

	for _, ext := range []string{"cube", "earthdistance"} {
		if err := db.Exec("CREATE EXTENSION IF NOT EXISTS " + ext).Error; err != nil {
			return fmt.Errorf("create extension %s: %w", ext, err)
		}
	}

	if err := db.AutoMigrate(&Street{}, &HouseNumber{}, &APIToken{}); err != nil {
		return err
	}

	// Serves the nearest-neighbour ordering in HouseNumberStore.Nearest.
	err := db.Exec("CREATE INDEX IF NOT EXISTS house_numbers_earth_idx ON house_numbers USING gist (ll_to_earth(latitude, longitude))").Error
	if err != nil {
		return fmt.Errorf("create house_numbers_earth_idx: %w", err)
	}

	return nil
}
