package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestUIRoutesBypassAuth checks that the web UI and its config load without a
// token (the user can't have one before signing in), while the API still
// requires one.
func TestUIRoutesBypassAuth(t *testing.T) {
	t.Parallel()

	ui := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-UI-Path", r.URL.Path)
	})
	cfg := UIConfig{
		OIDC:    UIOIDCConfig{Issuer: "https://issuer.example", ClientID: testAudience, Scope: "openid"},
		Version: "v1.2.3",
	}

	// A nil verifier is never reached: every request here either bypasses
	// auth or is rejected for its missing Authorization header first.
	handler := NewHandler(Config{UI: ui, UIConfig: cfg, CORSAllowedOrigin: "*"}, nil, nil, nil)

	tests := []struct {
		name     string
		path     string
		wantCode int
		wantPath string
	}{
		{"ui index", "/ui/", http.StatusOK, "/"},
		{"ui deep link", "/ui/streets/abc", http.StatusOK, "/streets/abc"},
		{"root redirects to ui", "/", http.StatusFound, ""},
		{"api requires token", "/api/streets", http.StatusUnauthorized, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}

			if got := rec.Header().Get("X-UI-Path"); got != tt.wantPath {
				t.Errorf("path seen by UI = %q, want %q", got, tt.wantPath)
			}
		})
	}

	t.Run("ui config", func(t *testing.T) {
		t.Parallel()

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/ui-config", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var got UIConfig
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}

		if got != cfg {
			t.Errorf("config = %+v, want %+v", got, cfg)
		}
	})
}
