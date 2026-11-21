package store

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// migrateLockKey identifies the advisory lock that serializes Migrate.
const migrateLockKey = 0x61646472 // "addr"

// Migrate creates the schema, or adds missing tables, columns and indexes to
// an existing one. It is safe to run on each startup.
//
// It needs the cube and earthdistance extensions, which are trusted, so a
// database owner without superuser rights can create them.
//
// Concurrent calls, from several instances starting at once or from parallel
// tests, run one after another: "IF NOT EXISTS" doesn't stop two sessions
// from racing to create the same extension or index.
func Migrate(ctx context.Context, db *gorm.DB) error {
	// The lock belongs to a session, so it and the migration share one pinned
	// connection.
	return db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("SELECT pg_advisory_lock(?)", migrateLockKey).Error; err != nil {
			return fmt.Errorf("lock migration: %w", err)
		}

		defer conn.Exec("SELECT pg_advisory_unlock(?)", migrateLockKey)

		return migrate(conn)
	})
}

func migrate(db *gorm.DB) error {
	for _, ext := range []string{"cube", "earthdistance"} {
		if err := db.Exec("CREATE EXTENSION IF NOT EXISTS " + ext).Error; err != nil {
			return fmt.Errorf("create extension %s: %w", ext, err)
		}
	}

	err := db.AutoMigrate(&Collector{}, &Street{}, &HouseNumber{}, &GeoObject{}, &APIToken{}, &CollectorRun{})
	if err != nil {
		return err
	}

	if err := migrateIndexes(db); err != nil {
		return err
	}

	if err := migrateGeoObjectGroupNameKey(db); err != nil {
		return err
	}

	return migrateHouseNumberKey(db)
}

// indexExistsQuery reports whether the named index exists in the schema that
// unqualified names create tables and indexes in. A bare to_regclass would
// search the whole search_path, and so could find another schema's index.
const indexExistsQuery = "SELECT to_regclass(quote_ident(current_schema()) || '.' || quote_ident(?)) IS NOT NULL"

// migrateIndexes creates the indexes AutoMigrate can't express.
func migrateIndexes(db *gorm.DB) error {
	indexes := []struct {
		name   string
		unique bool
		on     string
	}{
		// Serves the nearest-neighbour ordering in HouseNumberStore.Nearest.
		{"house_numbers_earth_idx", false, "house_numbers USING gist (ll_to_earth(latitude, longitude))"},
		// Serves the nearest-neighbour ordering in GeoObjectStore.Nearest.
		{"geo_objects_earth_idx", false, "geo_objects USING gist (ll_to_earth(latitude, longitude))"},
		// Lets other systems address their geo objects by GeoObject.ExternalID.
		{geoObjectsExternalIDKey, true, "geo_objects (external_id) WHERE external_id IS NOT NULL"},
		// Let collectors match their streets and house numbers by the IDs of
		// their source, so renames upstream update rows in place.
		{streetsExternalIDKey, true, "streets (external_id) WHERE external_id IS NOT NULL"},
		{houseNumbersExternalIDKey, true, "house_numbers (external_id) WHERE external_id IS NOT NULL"},
		// At most one active run per collector, even across restarts.
		{collectorRunsActiveKey, true, "collector_runs (collector_id) WHERE status = 'running'"},
		// Serves run history and its pruning.
		{"collector_runs_collector_started_idx", false, "collector_runs (collector_id, started_at DESC)"},
	}

	for _, idx := range indexes {
		create := "CREATE INDEX IF NOT EXISTS "
		if idx.unique {
			create = "CREATE UNIQUE INDEX IF NOT EXISTS "
		}

		if err := db.Exec(create + idx.name + " ON " + idx.on).Error; err != nil {
			return fmt.Errorf("create %s: %w", idx.name, err)
		}
	}

	return nil
}

// migrateHouseNumberKey makes a house number's number and addition unique on
// its street, so collectors can match house numbers by them. Databases from
// before this rule may hold duplicates; those are reported by address, to be
// fixed by hand before the next start.
func migrateHouseNumberKey(db *gorm.DB) error {
	var exists bool

	err := db.Raw(indexExistsQuery, houseNumbersStreetNumberKey).Scan(&exists).Error
	if err != nil {
		return fmt.Errorf("look up %s: %w", houseNumbersStreetNumberKey, err)
	}

	if exists {
		return nil
	}

	var duplicates []struct {
		Name           string
		District       string
		City           string
		Number         int
		NumberAddition string
		Count          int
	}

	err = db.Raw(`SELECT s.name, s.district, s.city, h.number,
			coalesce(h.number_addition, '') AS number_addition, count(*) AS count
		FROM house_numbers h JOIN streets s ON s.id = h.street_id
		GROUP BY s.id, s.name, s.district, s.city, h.number, coalesce(h.number_addition, '')
		HAVING count(*) > 1
		ORDER BY s.name, s.district, s.city, h.number LIMIT 10`).Scan(&duplicates).Error
	if err != nil {
		return fmt.Errorf("find duplicate house numbers: %w", err)
	}

	if len(duplicates) > 0 {
		names := make([]string, len(duplicates))
		for i, d := range duplicates {
			names[i] = fmt.Sprintf("%s %d%s (%s, %s; %d times)", d.Name, d.Number, d.NumberAddition, d.District, d.City, d.Count)
		}

		return fmt.Errorf("house numbers must be unique per street, but these are not (first %d shown): %s",
			len(names), strings.Join(names, ", "))
	}

	err = db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS " + houseNumbersStreetNumberKey +
		" ON house_numbers (street_id, number, coalesce(number_addition, ''))").Error
	if err != nil {
		return fmt.Errorf("create %s: %w", houseNumbersStreetNumberKey, err)
	}

	return nil
}

// migrateGeoObjectGroupNameKey makes geo object names unique per group. The
// index leads with group_name, so it also serves lookups by group.
//
// Databases from before this rule may hold duplicates, which would make the
// index fail with a terse error; those are reported by name instead, to be
// renamed or deleted by hand before the next start.
func migrateGeoObjectGroupNameKey(db *gorm.DB) error {
	var exists bool

	err := db.Raw(indexExistsQuery, geoObjectsGroupNameKey).Scan(&exists).Error
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
