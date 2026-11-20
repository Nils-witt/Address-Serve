package store

import (
	"context"
	"fmt"
	"strings"

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

	if err := db.AutoMigrate(&Collector{}, &CollectorRun{}, &Street{}, &HouseNumber{}, &GeoObject{}, &APIToken{}); err != nil {
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
	err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS " + geoObjectsExternalIDKey + " ON geo_objects (external_id) WHERE external_id IS NOT NULL").Error
	if err != nil {
		return fmt.Errorf("create %s: %w", geoObjectsExternalIDKey, err)
	}

	if err := migrateGeoObjectGroupNameKey(db); err != nil {
		return err
	}

	return migrateHouseNumbersKey(db)
}

// migrateGeoObjectGroupNameKey makes geo object names unique per group. The
// index leads with group_name, so it also serves lookups by group.
//
// Databases from before this rule may hold duplicates, which would make the
// index fail with a terse error; those are reported by name instead, to be
// renamed or deleted by hand before the next start.
func migrateGeoObjectGroupNameKey(db *gorm.DB) error {
	var exists bool

	err := db.Raw("SELECT to_regclass(?) IS NOT NULL", geoObjectsGroupNameKey).Scan(&exists).Error
	if err != nil {
		return fmt.Errorf("look up %s: %w", geoObjectsGroupNameKey, err)
	}

	if exists {
		return nil
	}

	var duplicates []struct {
		GroupName string
		Name      string
		Count     int
	}

	err = db.Raw(`SELECT group_name, name, count(*) AS count FROM geo_objects
		GROUP BY group_name, name HAVING count(*) > 1
		ORDER BY group_name, name LIMIT 10`).Scan(&duplicates).Error
	if err != nil {
		return fmt.Errorf("find duplicate geo object names: %w", err)
	}

	if len(duplicates) > 0 {
		names := make([]string, len(duplicates))
		for i, d := range duplicates {
			names[i] = fmt.Sprintf("%q in group %q (%d times)", d.Name, d.GroupName, d.Count)
		}

		return fmt.Errorf("geo object names must be unique per group, but these are not (first %d shown): %s",
			len(names), strings.Join(names, ", "))
	}

	err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS " + geoObjectsGroupNameKey + " ON geo_objects (group_name, name)").Error
	if err != nil {
		return fmt.Errorf("create %s: %w", geoObjectsGroupNameKey, err)
	}

	return nil
}

// migrateHouseNumbersKey makes a house number's number and addition unique
// per street, so collectors can upsert house numbers by them. A missing
// addition counts as the empty one.
//
// Like migrateGeoObjectGroupNameKey, it reports duplicates from before this
// rule by name instead of failing on the index, to be removed by hand before
// the next start.
func migrateHouseNumbersKey(db *gorm.DB) error {
	var exists bool

	err := db.Raw("SELECT to_regclass(?) IS NOT NULL", houseNumbersKey).Scan(&exists).Error
	if err != nil {
		return fmt.Errorf("look up %s: %w", houseNumbersKey, err)
	}

	if exists {
		return nil
	}

	var duplicates []struct {
		Street   string
		City     string
		Number   int
		Addition string
		Count    int
	}

	err = db.Raw(`SELECT s.name AS street, s.city, h.number, coalesce(h.number_addition, '') AS addition, count(*) AS count
		FROM house_numbers h JOIN streets s ON s.id = h.street_id
		GROUP BY s.id, s.name, s.city, h.number, coalesce(h.number_addition, '') HAVING count(*) > 1
		ORDER BY s.name, s.city, h.number LIMIT 10`).Scan(&duplicates).Error
	if err != nil {
		return fmt.Errorf("find duplicate house numbers: %w", err)
	}

	if len(duplicates) > 0 {
		names := make([]string, len(duplicates))
		for i, d := range duplicates {
			names[i] = fmt.Sprintf("%q %d%s in %q (%d times)", d.Street, d.Number, d.Addition, d.City, d.Count)
		}

		return fmt.Errorf("house numbers must be unique per street, but these are not (first %d shown): %s",
			len(names), strings.Join(names, ", "))
	}

	err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS " + houseNumbersKey +
		" ON house_numbers (street_id, number, (coalesce(number_addition, '')))").Error
	if err != nil {
		return fmt.Errorf("create %s: %w", houseNumbersKey, err)
	}

	return nil
}
