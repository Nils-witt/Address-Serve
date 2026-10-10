package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestResponsesAreGzipped checks that responses are compressed for clients
// that accept gzip, and sent as is to those that don't.
func TestResponsesAreGzipped(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Config{CORSAllowedOrigin: "*"}, nil, nil, nil, nil)

	tests := []struct {
		name           string
		acceptEncoding string
		wantEncoding   string
	}{
		{"gzip accepted", "gzip", "gzip"},
		{"no encoding accepted", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/docs/openapi.yaml", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			if got := rec.Header().Get("Content-Encoding"); got != tt.wantEncoding {
				t.Fatalf("Content-Encoding = %q, want %q", got, tt.wantEncoding)
			}

			var body io.Reader = rec.Body
			if tt.wantEncoding == "gzip" {
				zr, err := gzip.NewReader(rec.Body)
				if err != nil {
					t.Fatal(err)
				}

				body = zr
			}

			got, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(got, openapiSpec) {
				t.Errorf("decoded body is %d bytes, want the %d-byte spec", len(got), len(openapiSpec))
			}
		})
	}
}
