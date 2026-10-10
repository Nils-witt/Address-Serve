package api

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/store"
)

// TestAPITokenFlow creates an API token as an OIDC user, uses it, and revokes
// it, through the full handler against a real database.
func TestAPITokenFlow(t *testing.T) {
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

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	provider := newFakeProvider(t, key)

	oidc, err := NewOIDCVerifier(t.Context(), OIDCConfig{IssuerURL: provider.URL, Audience: testAudience})
	if err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(Config{OIDC: oidc}, store.NewStreetStore(db), nil, nil, store.NewAPITokenStore(db))

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":    provider.URL,
		"aud":    testAudience,
		subClaim: "flow-" + uuid.NewString(),
		"exp":    time.Now().Add(time.Hour).Unix(),
	})
	token.Header["kid"] = testKID

	userJWT, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	do := func(method, path, bearer, body string) *httptest.ResponseRecorder {
		t.Helper()

		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}

		req := httptest.NewRequestWithContext(t.Context(), method, path, r)
		req.Header.Set("Authorization", "Bearer "+bearer)

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		return rec
	}

	expect := func(rec *httptest.ResponseRecorder, want int, step string) {
		t.Helper()

		if rec.Code != want {
			t.Fatalf("%s: status = %d, want %d (body %s)", step, rec.Code, want, rec.Body)
		}
	}

	rec := do(http.MethodPost, "/api/tokens", userJWT, `{"name":"flow test"}`)
	expect(rec, http.StatusCreated, "create")

	var created createdAPIToken
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	expect(do(http.MethodGet, "/api/streets", created.Token, ""), http.StatusOK, "use token")
	expect(do(http.MethodGet, "/api/tokens", created.Token, ""), http.StatusForbidden, "list with token")
	expect(do(http.MethodPost, "/api/tokens", created.Token, `{"name":"x"}`), http.StatusForbidden, "create with token")

	rec = do(http.MethodGet, "/api/tokens", userJWT, "")
	expect(rec, http.StatusOK, "list")

	if strings.Contains(rec.Body.String(), created.Token) {
		t.Fatal("list response contains the token secret")
	}

	expect(do(http.MethodDelete, "/api/tokens/"+created.ID.String(), userJWT, ""), http.StatusNoContent, "revoke")
	expect(do(http.MethodGet, "/api/streets", created.Token, ""), http.StatusUnauthorized, "use revoked token")
}
