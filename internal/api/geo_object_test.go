package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGeoObjectHandlers_RejectBadRequests checks the 400 responses served
// before the store is reached, so no database is needed.
func TestGeoObjectHandlers_RejectBadRequests(t *testing.T) {
	t.Parallel()

	const (
		validStreet = `"streetId":"db94a625-aa11-4c37-b9ff-9706e792e7dd"`
		collection  = "/api/geo-objects"
	)

	tests := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		target  string
		body    string
	}{
		{name: "create with invalid json", handler: handleCreateGeoObject(nil), method: http.MethodPost, target: collection, body: "{"},
		{name: "create without name", handler: handleCreateGeoObject(nil), method: http.MethodPost, target: collection, body: `{"group":"g",` + validStreet + `}`},
		{name: "create without street", handler: handleCreateGeoObject(nil), method: http.MethodPost, target: collection, body: `{"name":"n","group":"g"}`},
		{name: "create with invalid house number id", handler: handleCreateGeoObject(nil), method: http.MethodPost, target: collection, body: `{"name":"n","group":"g",` + validStreet + `,"houseNumberId":"nope"}`},
		{name: "create with latitude out of range", handler: handleCreateGeoObject(nil), method: http.MethodPost, target: collection, body: `{"name":"n","group":"g",` + validStreet + `,"latitude":91}`},
		{name: "list with invalid street id", handler: handleListGeoObjects(nil), method: http.MethodGet, target: collection + "?streetId=nope"},
		{name: "list with invalid house number id", handler: handleListGeoObjects(nil), method: http.MethodGet, target: collection + "?houseNumberId=nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.target, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			tt.handler(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d (body %q)", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}
