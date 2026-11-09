package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nils-witt/address-serve/internal/store"
)

// TestHandleReverseGeocodeValidation checks that invalid query parameters
// are rejected before the store is queried, so a nil store suffices.
func TestHandleReverseGeocodeValidation(t *testing.T) {
	t.Parallel()

	handler := handleReverseGeocode(nil, nil)

	tests := []struct {
		name  string
		query string
	}{
		{name: "missing lat and lon", query: ""},
		{name: "missing lon", query: "lat=50.7"},
		{name: "missing lat", query: "lon=7.1"},
		{name: "lat not a number", query: "lat=north&lon=7.1"},
		{name: "lat out of range", query: "lat=90.5&lon=7.1"},
		{name: "lon out of range", query: "lat=50.7&lon=-180.1"},
		{name: "lat NaN", query: "lat=NaN&lon=7.1"},
		{name: "lon infinite", query: "lat=50.7&lon=Inf"},
		{name: "limit zero", query: "lat=50.7&lon=7.1&limit=0"},
		{name: "limit above maximum", query: "lat=50.7&lon=7.1&limit=101"},
		{name: "limit not an integer", query: "lat=50.7&lon=7.1&limit=abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/reverse?"+tt.query, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

// TestHandleReverseGeocodeResponse checks the response shape against a real
// database: both lists are always present and arrays, even when empty.
func TestHandleReverseGeocodeResponse(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("ADDRESS_SERV_TEST_DSN")
	if dsn == "" {
		t.Skip("ADDRESS_SERV_TEST_DSN not set")
	}

	db, err := store.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close(db) })

	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	handler := handleReverseGeocode(store.NewHouseNumberStore(db), store.NewGeoObjectStore(db))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/reverse?lat=50.7&lon=7.1&limit=3", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body)
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body, err)
	}

	for _, key := range []string{"houseNumbers", "geoObjects"} {
		var list []json.RawMessage

		raw, ok := body[key]
		if !ok || json.Unmarshal(raw, &list) != nil || list == nil {
			t.Errorf("%s = %s, want a JSON array", key, raw)
		}
	}
}
