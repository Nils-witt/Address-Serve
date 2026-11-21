package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const (
	adminRole   = "collector-admin"
	groupsClaim = "groups"
)

func TestClaimContains(t *testing.T) {
	t.Parallel()

	claims := map[string]any{
		"role":         adminRole,
		groupsClaim:    []any{"users", adminRole},
		"realm_access": map[string]any{"roles": []any{"offline", adminRole}},
		"numbers":      []any{1, 2},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"role", true},
		{groupsClaim, true},
		{"realm_access.roles", true},
		{"realm_access", false},
		{"realm_access.roles.extra", false},
		{"numbers", false},
		{"missing", false},
		{"role.sub", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			if got := claimContains(claims, tt.path, adminRole); got != tt.want {
				t.Errorf("claimContains(%s) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}

func TestRequireAdmin(t *testing.T) {
	t.Parallel()

	admin := AdminClaim{Claim: groupsClaim, Value: "admins"}
	tokenID := uuid.New()

	tests := []struct {
		name     string
		claim    AdminClaim
		p        principal
		wantCode int
		wantBody string
	}{
		{name: "admin", claim: admin, p: principal{Claims: map[string]any{groupsClaim: []any{"admins"}}}, wantCode: http.StatusOK},
		{name: "not configured", p: principal{Claims: map[string]any{groupsClaim: []any{"admins"}}}, wantCode: http.StatusForbidden, wantBody: "disabled"},
		{name: "without the claim", claim: admin, p: principal{Claims: map[string]any{groupsClaim: []any{"users"}}}, wantCode: http.StatusForbidden, wantBody: "requires the claim groups"},
		{name: "API token", claim: admin, p: principal{APITokenID: &tokenID}, wantCode: http.StatusForbidden, wantBody: "API tokens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := requireAdmin(tt.claim, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			ctx := context.WithValue(t.Context(), principalContextKey, tt.p)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/collectors", nil))

			if rec.Code != tt.wantCode || !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("response = %d %s, want %d containing %q", rec.Code, rec.Body, tt.wantCode, tt.wantBody)
			}
		})
	}
}

// apiClient sends authenticated requests to a handler and decodes replies.
type apiClient struct {
	t       *testing.T
	handler http.Handler
}

func (c apiClient) do(method, path, bearer, body string) *httptest.ResponseRecorder {
	c.t.Helper()

	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}

	req := httptest.NewRequestWithContext(c.t.Context(), method, path, r)
	req.Header.Set("Authorization", "Bearer "+bearer)

	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	return rec
}

// expect sends a request, fails the test unless it gets want, and decodes
// the reply into out, if not nil.
func (c apiClient) expect(method, path, bearer, body string, want int, out any) string {
	c.t.Helper()

	rec := c.do(method, path, bearer, body)
	if rec.Code != want {
		c.t.Fatalf("%s %s: status = %d, want %d (body %s)", method, path, rec.Code, want, rec.Body)
	}

	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			c.t.Fatalf("%s %s: decode %s: %v", method, path, rec.Body, err)
		}
	}

	return rec.Body.String()
}
