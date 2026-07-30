package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func openDB() (*sql.DB, error) {
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
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
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
	postcode TEXT NOT NULL,
	latitude DOUBLE PRECISION NOT NULL,
	longitude DOUBLE PRECISION NOT NULL,
	UNIQUE (name, district, city)
);`

// Adds the uniqueness constraint to databases created before it was part of
// createStreetsTable. The constraint name matches Postgres's default naming
// so this is a no-op once the constraint already exists.
const addStreetsUniqueConstraint = `
DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint WHERE conname = 'streets_name_district_city_key'
	) THEN
		ALTER TABLE streets ADD CONSTRAINT streets_name_district_city_key UNIQUE (name, district, city);
	END IF;
END $$;`

const createHouseNumbersTable = `
CREATE TABLE IF NOT EXISTS house_numbers (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	street_id UUID NOT NULL REFERENCES streets(id) ON DELETE CASCADE,
	number INTEGER NOT NULL,
	number_addition TEXT,
	latitude DOUBLE PRECISION NOT NULL,
	longitude DOUBLE PRECISION NOT NULL
);`

func migrate(db *sql.DB) error {
	if _, err := db.Exec(createStreetsTable); err != nil {
		return err
	}
	if _, err := db.Exec(addStreetsUniqueConstraint); err != nil {
		return err
	}
	_, err := db.Exec(createHouseNumbersTable)
	return err
}
