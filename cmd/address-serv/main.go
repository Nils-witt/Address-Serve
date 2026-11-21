// Command address-serv serves the address lookup and management HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	"github.com/nils-witt/address-serve/frontend"
	"github.com/nils-witt/address-serve/internal/api"
	"github.com/nils-witt/address-serve/internal/api/spa"
	"github.com/nils-witt/address-serve/internal/collector"
	"github.com/nils-witt/address-serve/internal/store"
)

// Build info, set at link time with -X (see Dockerfile and .goreleaser.yml).
var (
	version = "dev"
	commit  = "none"
	branch  string
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

	collectors, err := setUpCollectors(ctx, store.NewCollectorStore(db))
	if err != nil {
		return err
	}

	handler := api.NewHandler(
		api.Config{
			OIDC:              oidc,
			CORSAllowedOrigin: getenvDefault("CORS_ALLOWED_ORIGIN", "*"),
			UI:                ui,
			UIConfig:          uiConfig(),
			Collectors:        collectors,
		},
		store.NewStreetStore(db),
		store.NewHouseNumberStore(db),
		store.NewGeoObjectStore(db),
		store.NewBulkStore(db),
		store.NewAPITokenStore(db),
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

// setUpCollectors configures the collector runner from the COLLECTOR*
// variables, fails the runs a previous process left running, and scans the
// collectors folder once. A folder that can't be scanned is logged, not
// fatal: it can be fixed and reloaded from the UI.
func setUpCollectors(ctx context.Context, cs *store.CollectorStore) (api.CollectorConfig, error) {
	if !collector.Supported {
		log.Print("collectors are not supported on this OS; their routes are disabled")
		return api.CollectorConfig{}, nil
	}

	cfg, err := collectorConfig()
	if err != nil {
		return api.CollectorConfig{}, err
	}

	if n, err := cs.FailInterruptedRuns(ctx); err != nil {
		return api.CollectorConfig{}, fmt.Errorf("failed to recover collector runs: %w", err)
	} else if n > 0 {
		log.Printf("marked %d collector runs interrupted by the last shutdown as failed", n)
	}

	runner := collector.NewRunner(cs, cfg)

	found, err := runner.Reload(ctx)
	if err != nil {
		log.Printf("collectors: %v", err)
	} else {
		log.Printf("collectors: %d known, folder %s", len(found), cfg.Dir)
	}

	admin := api.AdminClaim{Claim: os.Getenv("COLLECTOR_ADMIN_CLAIM"), Value: os.Getenv("COLLECTOR_ADMIN_VALUE")}
	if admin.Claim == "" || admin.Value == "" {
		log.Print("COLLECTOR_ADMIN_CLAIM or COLLECTOR_ADMIN_VALUE is unset; nobody can manage collectors")
	}

	return api.CollectorConfig{Runner: runner, Store: cs, Admin: admin}, nil
}

// collectorConfig reads the runner's settings from the environment.
func collectorConfig() (collector.Config, error) {
	cfg := collector.Config{Dir: os.Getenv("COLLECTORS_DIR")}

	if cfg.Dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return collector.Config{}, fmt.Errorf("locate the collectors folder: %w; set COLLECTORS_DIR", err)
		}

		cfg.Dir = filepath.Join(filepath.Dir(exe), "collectors")
	}

	var err error

	if cfg.Timeout, err = time.ParseDuration(getenvDefault("COLLECTOR_TIMEOUT", "10m")); err != nil || cfg.Timeout <= 0 {
		return collector.Config{}, errors.New("COLLECTOR_TIMEOUT must be a positive duration such as 10m")
	}

	cfg.GeoMaxDistanceMeters, err = strconv.ParseFloat(getenvDefault("COLLECTOR_GEO_MAX_DISTANCE_M", "100"), 64)
	if err != nil || cfg.GeoMaxDistanceMeters < 0 {
		return collector.Config{}, errors.New("COLLECTOR_GEO_MAX_DISTANCE_M must be a non-negative number of metres")
	}

	if cfg.RunHistory, err = strconv.Atoi(getenvDefault("COLLECTOR_RUN_HISTORY", "50")); err != nil || cfg.RunHistory < 1 {
		return collector.Config{}, errors.New("COLLECTOR_RUN_HISTORY must be a positive integer")
	}

	return cfg, nil
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
		Branch:  branch,
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
