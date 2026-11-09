package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleReverseGeocodeValidation checks that invalid query parameters
// are rejected before the store is queried, so a nil store suffices.
func TestHandleReverseGeocodeValidation(t *testing.T) {
	t.Parallel()

	handler := handleReverseGeocode(nil)

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
