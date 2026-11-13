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

	if err := db.AutoMigrate(&Street{}, &HouseNumber{}, &GeoObject{}, &APIToken{}); err != nil {
		return err
	}

	// Serves the nearest-neighbour ordering in HouseNumberStore.Nearest.
	err := db.Exec("CREATE INDEX IF NOT EXISTS house_numbers_earth_idx ON house_numbers USING gist (ll_to_earth(latitude, longitude))").Error
	if err != nil {
		return fmt.Errorf("create house_numbers_earth_idx: %w", err)
	}

	// Serves the nearest-neighbour ordering in GeoObjectStore.Nearest.
	err = db.Exec("CREATE INDEX IF NOT EXISTS geo_objects_earth_idx ON geo_objects USING gist (ll_to_earth(latitude, longitude))").Error
	if err != nil {
		return fmt.Errorf("create geo_objects_earth_idx: %w", err)
	}

	// Lets other systems address their geo objects by GeoObject.ExternalID.
	err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS geo_objects_external_id_key ON geo_objects (external_id) WHERE external_id IS NOT NULL").Error
	if err != nil {
		return fmt.Errorf("create geo_objects_external_id_key: %w", err)
	}

	return nil
}
