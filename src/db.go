package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func openDB(ctx context.Context) (*sql.DB, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		host := getenvDefault("POSTGRES_HOST", "localhost")
		port := getenvDefault("POSTGRES_PORT", "5432")
		user := getenvDefault("POSTGRES_USER", "address_serv")
		password := getenvDefault("POSTGRES_PASSWORD", "address_serv")
		dbname := getenvDefault("POSTGRES_DB", "address_serv")
		sslmode := getenvDefault("POSTGRES_SSLMODE", "disable")
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, password, host, port, dbname, sslmode)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	return db, nil
}

// execUpdate runs an UPDATE statement, mapping a constraint violation
// recognized by isConflict to conflictErr and a zero row count to
// sql.ErrNoRows.
func execUpdate(ctx context.Context, db *sql.DB, query string, args []any, isConflict func(error) bool, conflictErr error) error {
	res, err := db.ExecContext(ctx, query, args...)
	if isConflict(err) {
		return conflictErr
	}

	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

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

func migrate(ctx context.Context, db *sql.DB) error {
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
