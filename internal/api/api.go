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
	// UI serves the embedded web UI under /ui/. Nil disables the UI routes.
	UI http.Handler
	// UIConfig is served to the web UI at GET /ui-config.
	UIConfig UIConfig
}

// NewHandler returns the root handler serving every API route, wrapped in
// logging, CORS and auth middleware. The web UI and its config are the only
// routes served without auth.
func NewHandler(cfg Config, streets *store.StreetStore, houseNumbers *store.HouseNumberStore, bulk *store.BulkStore) http.Handler {
	mux := http.NewServeMux()
	registerStreetRoutes(mux, streets)
	registerHouseNumberRoutes(mux, houseNumbers)
	registerBulkRoutes(mux, bulk)
	registerDocsRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	root := http.NewServeMux()
	if cfg.UI != nil {
		registerUIRoutes(root, cfg.UI, cfg.UIConfig)
	}

	root.Handle("/", authMiddleware(cfg.OIDC)(mux))

	return loggingMiddleware(corsMiddleware(cfg.CORSAllowedOrigin)(root))
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
