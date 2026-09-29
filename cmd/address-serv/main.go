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

	"github.com/nils-witt/address-serve/frontend"
	"github.com/nils-witt/address-serve/internal/api"
	"github.com/nils-witt/address-serve/internal/api/spa"
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

	ctx := context.Background()

	oidc, err := oidcVerifier(ctx)
	if err != nil {
		return err
	}

	db, err := store.Open(ctx, databaseURL())
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = store.Close(db) }()

	if err := store.Migrate(ctx, db); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	ui, err := spa.Handler(frontend.DistFS)
	if err != nil {
		return fmt.Errorf("failed to load web UI: %w", err)
	}

	handler := api.NewHandler(
		api.Config{
			OIDC:              oidc,
			CORSAllowedOrigin: getenvDefault("CORS_ALLOWED_ORIGIN", "*"),
			UI:                ui,
			UIConfig:          uiConfig(),
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

// oidcVerifier returns a verifier for tokens issued by the OpenID Connect
// provider at OIDC_ISSUER_URL.
func oidcVerifier(ctx context.Context) (*api.OIDCVerifier, error) {
	issuer := os.Getenv("OIDC_ISSUER_URL")
	if issuer == "" {
		return nil, errors.New("OIDC_ISSUER_URL environment variable must be set")
	}

	verifier, err := api.NewOIDCVerifier(ctx, api.OIDCConfig{
		IssuerURL: issuer,
		Audience:  os.Getenv("OIDC_AUDIENCE"),
		JWKSURL:   os.Getenv("OIDC_JWKS_URL"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set up OIDC: %w", err)
	}

	return verifier, nil
}

// uiConfig describes the OIDC client the web UI signs in with. The client ID
// defaults to OIDC_AUDIENCE, which fits providers where a public client's own
// ID ends up in the access token's audience.
func uiConfig() api.UIConfig {
	return api.UIConfig{
		OIDC: api.UIOIDCConfig{
			Issuer:   os.Getenv("OIDC_ISSUER_URL"),
			ClientID: getenvDefault("OIDC_CLIENT_ID", os.Getenv("OIDC_AUDIENCE")),
			Scope:    getenvDefault("OIDC_SCOPE", "openid profile email"),
		},
		Version: version,
		Commit:  commit,
	}
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
