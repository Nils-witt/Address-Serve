package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/store"
)

// fakeAPITokens authenticates the secrets it holds as keys.
type fakeAPITokens map[string]store.APIToken

func (f fakeAPITokens) Authenticate(_ context.Context, secret string) (store.APIToken, error) {
	t, ok := f[secret]
	if !ok {
		return store.APIToken{}, store.ErrInvalidAPIToken
	}

	return t, nil
}

const (
	testAPIToken = store.APITokenPrefix + "test-secret"
	testAudience = "address-serv"
	testKID      = "test-key"
	testSubject  = "user-1"
	subClaim     = "sub"
)

// newFakeProvider serves an OpenID discovery document and a JWKS holding the
// public half of key.
func newFakeProvider(t *testing.T, key *rsa.PrivateKey) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"issuer":   srv.URL,
			"jwks_uri": srv.URL + "/jwks",
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": testKID,
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	})

	return srv
}

func TestAuthMiddleware(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	provider := newFakeProvider(t, key)

	oidc, err := NewOIDCVerifier(t.Context(), OIDCConfig{IssuerURL: provider.URL, Audience: testAudience})
	if err != nil {
		t.Fatal(err)
	}

	tokens := fakeAPITokens{testAPIToken: {ID: uuid.New(), OwnerSub: testSubject, OwnerName: "User One"}}

	handler := authMiddleware(oidc, tokens)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := principalFrom(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{subClaim: p.Subject})
	}))

	validClaims := func() jwt.MapClaims {
		return jwt.MapClaims{
			"iss":    provider.URL,
			"aud":    testAudience,
			subClaim: testSubject,
			"exp":    time.Now().Add(time.Hour).Unix(),
		}
	}

	signRSA := func(k *rsa.PrivateKey, claims jwt.MapClaims) string {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		token.Header["kid"] = testKID

		s, err := token.SignedString(k)
		if err != nil {
			t.Fatal(err)
		}

		return s
	}

	signHMAC := func(secret []byte, claims jwt.MapClaims) string {
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
		if err != nil {
			t.Fatal(err)
		}

		return s
	}

	with := func(mutate func(jwt.MapClaims)) jwt.MapClaims {
		c := validClaims()
		mutate(c)

		return c
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	tests := []struct {
		name  string
		token string
		want  int
	}{
		{"oidc valid", signRSA(key, validClaims()), http.StatusOK},
		{"oidc wrong audience", signRSA(key, with(func(c jwt.MapClaims) { c["aud"] = "other" })), http.StatusUnauthorized},
		{"oidc wrong issuer", signRSA(key, with(func(c jwt.MapClaims) { c["iss"] = "https://evil.example" })), http.StatusUnauthorized},
		{"oidc expired", signRSA(key, with(func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() })), http.StatusUnauthorized},
		{"oidc missing exp", signRSA(key, with(func(c jwt.MapClaims) { delete(c, "exp") })), http.StatusUnauthorized},
		{"oidc wrong key", signRSA(otherKey, validClaims()), http.StatusUnauthorized},
		{"hmac shared secret", signHMAC([]byte("test-secret"), validClaims()), http.StatusUnauthorized},
		{"hmac signed with public key", signHMAC(pubPEM, validClaims()), http.StatusUnauthorized},
		{"garbage", "not-a-jwt", http.StatusUnauthorized},
		{"api token valid", testAPIToken, http.StatusOK},
		{"api token unknown", store.APITokenPrefix + "unknown", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body)
			}

			if tt.want == http.StatusOK {
				var body map[string]any
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}

				if body[subClaim] != testSubject {
					t.Fatalf("claims not passed to handler: %v", body)
				}
			}
		})
	}
}

func TestNewOIDCVerifierIssuerMismatch(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	provider := newFakeProvider(t, key)

	_, err = NewOIDCVerifier(t.Context(), OIDCConfig{IssuerURL: provider.URL + "/", Audience: testAudience})
	if err == nil {
		t.Fatal("expected issuer mismatch error")
	}
}

func TestRequireTokenOwner(t *testing.T) {
	t.Parallel()

	tokenID := uuid.New()

	tests := []struct {
		name      string
		principal *principal
		want      int
	}{
		{"oidc user", &principal{Subject: testSubject}, http.StatusOK},
		{"api token", &principal{Subject: testSubject, APITokenID: &tokenID}, http.StatusForbidden},
		{"oidc without subject", &principal{}, http.StatusForbidden},
		{"unauthenticated", nil, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			if tt.principal != nil {
				ctx = context.WithValue(ctx, principalContextKey, *tt.principal)
			}

			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/tokens", nil)
			rec := httptest.NewRecorder()

			if _, ok := requireTokenOwner(rec, req); ok {
				rec.WriteHeader(http.StatusOK)
			}

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestCreateAPITokenRequestValidate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name    string
		req     createAPITokenRequest
		wantErr bool
	}{
		{"name only", createAPITokenRequest{Name: " ci "}, false},
		{"future expiry", createAPITokenRequest{Name: "ci", ExpiresAt: &future}, false},
		{"blank name", createAPITokenRequest{Name: "  "}, true},
		{"long name", createAPITokenRequest{Name: strings.Repeat("x", maxAPITokenNameLen+1)}, true},
		{"past expiry", createAPITokenRequest{Name: "ci", ExpiresAt: &past}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.req.validate(now); (err != nil) != tt.wantErr {
				t.Fatalf("validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
