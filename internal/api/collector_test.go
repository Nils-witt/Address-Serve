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

// adminGroup is the group the tests let manage collectors.
const adminGroup = "collector-admins"

func TestInGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		claims map[string]any
		want   bool
	}{
		{name: "listed", claims: map[string]any{groupsClaim: []any{"users", adminGroup}}, want: true},
		{name: "the only group, as a string", claims: map[string]any{groupsClaim: adminGroup}, want: true},
		{name: "other groups", claims: map[string]any{groupsClaim: []any{"users"}}},
		{name: "no groups claim", claims: map[string]any{"roles": []any{adminGroup}}},
		{name: "not strings", claims: map[string]any{groupsClaim: []any{1, 2}}},
		{name: "case differs", claims: map[string]any{groupsClaim: []any{"Collector-Admins"}}},
		{name: "no claims at all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := inGroup(tt.claims, adminGroup); got != tt.want {
				t.Errorf("inGroup = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestRequireAdminGroup(t *testing.T) {
	t.Parallel()

	tokenID := uuid.New()
	member := principal{Claims: map[string]any{groupsClaim: []any{adminGroup}}}

	tests := []struct {
		name     string
		group    string
		p        principal
		wantCode int
		wantBody string
	}{
		{name: "member", group: adminGroup, p: member, wantCode: http.StatusOK},
		{name: "not configured", p: member, wantCode: http.StatusForbidden, wantBody: "set COLLECTOR_ADMIN_GROUP"},
		{name: "not a member", group: adminGroup, p: principal{Claims: map[string]any{groupsClaim: []any{"users"}}}, wantCode: http.StatusForbidden, wantBody: "membership in the group"},
		{name: "API token", group: adminGroup, p: principal{APITokenID: &tokenID}, wantCode: http.StatusForbidden, wantBody: "API tokens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := requireAdminGroup(tt.group, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
