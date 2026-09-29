package store

import (
	"context"
	"database/sql"
)

const createStreetsTable = `
CREATE TABLE IF NOT EXISTS streets (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	city TEXT NOT NULL,
	district TEXT NOT NULL,
	name TEXT NOT NULL,
	country TEXT NOT NULL,
	latitude DOUBLE PRECISION NOT NULL,
	longitude DOUBLE PRECISION NOT NULL,
	UNIQUE (name, district, city, country)
);`

// Drops the postcode column from databases created before postcode moved to
// house_numbers. No-op once the column is already gone.
const dropStreetsPostcodeColumn = `
ALTER TABLE streets DROP COLUMN IF EXISTS postcode;`

// Adds the country column to databases created before it was part of
// createStreetsTable. The default is dropped immediately after backfilling
// existing rows so new inserts must supply a country explicitly.
const addStreetsCountryColumn = `
ALTER TABLE streets ADD COLUMN IF NOT EXISTS country TEXT NOT NULL DEFAULT '';
ALTER TABLE streets ALTER COLUMN country DROP DEFAULT;`

// Adds the uniqueness constraint to databases created before it was part of
// createStreetsTable, replacing the older constraint that predates the
// country column. Both checks make this a no-op once already applied.
const addStreetsUniqueConstraint = `
DO $$
BEGIN
	IF EXISTS (
		SELECT 1 FROM pg_constraint WHERE conname = 'streets_name_district_city_key'
	) THEN
		ALTER TABLE streets DROP CONSTRAINT streets_name_district_city_key;
	END IF;
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint WHERE conname = 'streets_name_district_city_country_key'
	) THEN
		ALTER TABLE streets ADD CONSTRAINT streets_name_district_city_country_key UNIQUE (name, district, city, country);
	END IF;
END $$;`

const createHouseNumbersTable = `
CREATE TABLE IF NOT EXISTS house_numbers (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	street_id UUID NOT NULL REFERENCES streets(id) ON DELETE CASCADE,
	number INTEGER NOT NULL,
	number_addition TEXT,
	postcode TEXT NOT NULL,
	latitude DOUBLE PRECISION NOT NULL,
	longitude DOUBLE PRECISION NOT NULL
);`

// Adds the postcode column to databases created before it moved here from
// streets. The default is dropped immediately after backfilling existing
// rows so new inserts must supply a postcode explicitly.
const addHouseNumbersPostcodeColumn = `
ALTER TABLE house_numbers ADD COLUMN IF NOT EXISTS postcode TEXT NOT NULL DEFAULT '';
ALTER TABLE house_numbers ALTER COLUMN postcode DROP DEFAULT;`

// Migrate creates or upgrades the schema. Every statement is idempotent, so it
// is safe to run on each startup.
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, createStreetsTable); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, addStreetsCountryColumn); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, addStreetsUniqueConstraint); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, dropStreetsPostcodeColumn); err != nil {
		return err
	}

	if _, err := db.ExecContext(ctx, createHouseNumbersTable); err != nil {
		return err
	}

	_, err := db.ExecContext(ctx, addHouseNumbersPostcodeColumn)

	return err
}
