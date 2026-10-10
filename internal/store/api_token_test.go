package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// createAPIToken creates a token for a fresh owner and deletes it when t
// ends. It returns the token, its secret and the owner.
func createAPIToken(t *testing.T, tokens *APITokenStore, expiresAt *time.Time) (APIToken, string, string) {
	t.Helper()

	owner := "owner-" + uuid.NewString()

	created, secret, err := tokens.Create(t.Context(), owner, "Owner", "ci", expiresAt)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = tokens.Delete(context.Background(), owner, created.ID) })

	return created, secret, owner
}

func TestAPITokenAuthenticate(t *testing.T) {
	t.Parallel()

	tokens := NewAPITokenStore(openBenchDB(t))
	created, secret, owner := createAPIToken(t, tokens, nil)

	if !strings.HasPrefix(secret, APITokenPrefix) || !strings.HasPrefix(secret, created.Prefix) {
		t.Fatalf("secret %q does not start with %q and prefix %q", secret, APITokenPrefix, created.Prefix)
	}

	got, err := tokens.Authenticate(t.Context(), secret)
	if err != nil {
		t.Fatal(err)
	}

	if got.ID != created.ID || got.OwnerSub != owner || got.LastUsedAt == nil {
		t.Fatalf("Authenticate = %+v, want token %s of %s with last use recorded", got, created.ID, owner)
	}

	if _, err := tokens.Authenticate(t.Context(), secret+"x"); !errors.Is(err, ErrInvalidAPIToken) {
		t.Fatalf("wrong secret: err = %v, want ErrInvalidAPIToken", err)
	}
}

func TestAPITokenListAndDelete(t *testing.T) {
	t.Parallel()

	tokens := NewAPITokenStore(openBenchDB(t))
	created, secret, owner := createAPIToken(t, tokens, nil)

	list, err := tokens.List(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}

	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("List = %+v, want only %s", list, created.ID)
	}

	if err := tokens.Delete(t.Context(), "someone-else", created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete by other owner: err = %v, want ErrNotFound", err)
	}

	if err := tokens.Delete(t.Context(), owner, created.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := tokens.Authenticate(t.Context(), secret); !errors.Is(err, ErrInvalidAPIToken) {
		t.Fatalf("deleted token: err = %v, want ErrInvalidAPIToken", err)
	}
}

func TestAPITokenExpired(t *testing.T) {
	t.Parallel()

	tokens := NewAPITokenStore(openBenchDB(t))
	past := time.Now().Add(-time.Minute)
	_, secret, _ := createAPIToken(t, tokens, &past)

	if _, err := tokens.Authenticate(t.Context(), secret); !errors.Is(err, ErrInvalidAPIToken) {
		t.Fatalf("err = %v, want ErrInvalidAPIToken", err)
	}
}
