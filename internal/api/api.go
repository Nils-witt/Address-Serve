// Package api implements the address-serv HTTP API.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/store"
)

// Config holds the settings NewHandler needs besides the stores.
type Config struct {
	// OIDC verifies bearer tokens issued by an OpenID Connect provider.
	OIDC              *OIDCVerifier
	CORSAllowedOrigin string
}

// NewHandler returns the root handler serving every API route, wrapped in
// logging, CORS and auth middleware.
func NewHandler(cfg Config, streets *store.StreetStore, houseNumbers *store.HouseNumberStore, bulk *store.BulkStore) http.Handler {
	mux := http.NewServeMux()
	registerStreetRoutes(mux, streets)
	registerHouseNumberRoutes(mux, houseNumbers)
	registerBulkRoutes(mux, bulk)
	registerDocsRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return loggingMiddleware(corsMiddleware(cfg.CORSAllowedOrigin)(authMiddleware(cfg.OIDC)(mux)))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func idFromRequest(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(r.PathValue("id"))
}
