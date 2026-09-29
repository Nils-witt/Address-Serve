package spa

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandler(t *testing.T) {
	t.Parallel()

	built := fstest.MapFS{
		"dist/index.html":         {Data: []byte("<html>app</html>")},
		"dist/assets/app-1234.js": {Data: []byte("console.log(1)")},
	}

	tests := []struct {
		name     string
		fs       fstest.MapFS
		path     string
		wantCode int
		wantBody string
	}{
		{"root serves index", built, "/", http.StatusOK, "<html>app</html>"},
		{"asset served as file", built, "/assets/app-1234.js", http.StatusOK, "console.log(1)"},
		{"client route falls back to index", built, "/streets/abc", http.StatusOK, "<html>app</html>"},
		{"directory falls back to index", built, "/assets/", http.StatusOK, "<html>app</html>"},
		{"unbuilt UI explains itself", fstest.MapFS{"dist/.gitkeep": {}}, "/", http.StatusServiceUnavailable, "has not been built"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler, err := Handler(tt.fs)
			if err != nil {
				t.Fatal(err)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}

			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
