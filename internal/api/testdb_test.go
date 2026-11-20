package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"github.com/nils-witt/address-serve/internal/store"
)

var (
	migrateOnce sync.Once
	errMigrate  error
)

// openTestDB connects to the database at ADDRESS_SERV_TEST_DSN, or skips t
// if it is unset. The schema is migrated once per test binary: parallel
// migrations of a fresh database race each other.
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("ADDRESS_SERV_TEST_DSN")
	if dsn == "" {
		t.Skip("ADDRESS_SERV_TEST_DSN not set")
	}

	db, err := store.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close(db) })

	migrateOnce.Do(func() { errMigrate = store.Migrate(context.Background(), db) })

	if errMigrate != nil {
		t.Fatal(errMigrate)
	}

	return db
}

// newTestOIDC starts a fake OpenID Connect provider and returns a verifier
// for it and a function signing a valid access token for a subject.
func newTestOIDC(t *testing.T) (*OIDCVerifier, func(sub string) string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	provider := newFakeProvider(t, key)

	oidc, err := NewOIDCVerifier(t.Context(), OIDCConfig{IssuerURL: provider.URL, Audience: testAudience})
	if err != nil {
		t.Fatal(err)
	}

	sign := func(sub string) string {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss":    provider.URL,
			"aud":    testAudience,
			subClaim: sub,
			"exp":    time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = testKID

		s, err := token.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}

		return s
	}

	return oidc, sign
}
