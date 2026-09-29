// Command address-serv serves the address lookup and management HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/nils-witt/address-serve/internal/api"
	"github.com/nils-witt/address-serve/internal/store"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil {
		log.Print("no .env file found, relying on environment variables")
	}

	log.Printf("address-serv %s (%s)", version, commit)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return errors.New("JWT_SECRET environment variable must be set")
	}

	ctx := context.Background()

	db, err := store.Open(ctx, databaseURL())
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := store.Migrate(ctx, db); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	handler := api.NewHandler(
		api.Config{
			JWTSecret:         []byte(jwtSecret),
			CORSAllowedOrigin: getenvDefault("CORS_ALLOWED_ORIGIN", "*"),
		},
		store.NewStreetStore(db),
		store.NewHouseNumberStore(db),
		store.NewBulkStore(db),
	)

	addr := getenvDefault("LISTEN_ADDR", ":8080")
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("listening on %s", addr)

	return server.ListenAndServe()
}

// databaseURL returns DATABASE_URL, or builds a DSN from the POSTGRES_*
// variables when it is unset.
func databaseURL() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}

	host := getenvDefault("POSTGRES_HOST", "localhost")
	port := getenvDefault("POSTGRES_PORT", "5432")
	user := getenvDefault("POSTGRES_USER", "address_serv")
	password := getenvDefault("POSTGRES_PASSWORD", "address_serv")
	dbname := getenvDefault("POSTGRES_DB", "address_serv")
	sslmode := getenvDefault("POSTGRES_SSLMODE", "disable")

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, password, host, port, dbname, sslmode)
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
