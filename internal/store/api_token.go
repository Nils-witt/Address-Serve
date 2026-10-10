package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// APITokenPrefix starts every API token, so the auth middleware can tell one
// from an OIDC JWT without a database lookup.
const APITokenPrefix = "asv_"

// apiTokenDisplayLen is how many leading characters of a token are kept in
// clear, enough for the owner to recognise it in a list.
const apiTokenDisplayLen = len(APITokenPrefix) + 8

// lastUsedGranularity bounds how often a token's last_used_at is written, so
// a busy client doesn't turn every read into a write.
const lastUsedGranularity = time.Minute

// APIToken is a long-lived bearer token owned by an OIDC user, for scripts and
// other clients that can't sign in interactively. Only a SHA-256 hash of the
// secret is stored; the secret itself is shown once, when it is created.
type APIToken struct {
	ID         uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name       string     `json:"name" gorm:"not null"`
	OwnerSub   string     `json:"-" gorm:"not null;index"`
	OwnerName  string     `json:"-" gorm:"not null"`
	TokenHash  []byte     `json:"-" gorm:"not null;uniqueIndex"`
	Prefix     string     `json:"prefix" gorm:"not null"`
	CreatedAt  time.Time  `json:"createdAt" gorm:"not null"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

// Expired reports whether t has an expiry at or before now.
func (t APIToken) Expired(now time.Time) bool {
	return t.ExpiresAt != nil && !now.Before(*t.ExpiresAt)
}

// ErrInvalidAPIToken is returned when a presented token is unknown or expired.
var ErrInvalidAPIToken = errors.New("invalid or expired API token")

// APITokenStore reads and writes API tokens.
type APITokenStore struct {
	db *gorm.DB
}

// NewAPITokenStore returns an APITokenStore backed by db.
func NewAPITokenStore(db *gorm.DB) *APITokenStore {
	return &APITokenStore{db: db}
}

// Create generates a new token for the given owner and stores its hash. It
// returns the stored token and the secret, which can't be recovered later.
func (st *APITokenStore) Create(ctx context.Context, ownerSub, ownerName, name string, expiresAt *time.Time) (APIToken, string, error) {
	secret, err := newAPITokenSecret()
	if err != nil {
		return APIToken{}, "", err
	}

	t := APIToken{
		Name:      name,
		OwnerSub:  ownerSub,
		OwnerName: ownerName,
		TokenHash: hashAPIToken(secret),
		Prefix:    secret[:apiTokenDisplayLen],
		CreatedAt: time.Now().UTC(),
		ExpiresAt: expiresAt,
	}

	if err := st.db.WithContext(ctx).Create(&t).Error; err != nil {
		return APIToken{}, "", err
	}

	return t, secret, nil
}

// List returns the tokens of ownerSub, newest first.
func (st *APITokenStore) List(ctx context.Context, ownerSub string) ([]APIToken, error) {
	tokens := []APIToken{}
	err := st.db.WithContext(ctx).Where("owner_sub = ?", ownerSub).Order("created_at DESC").Find(&tokens).Error

	return tokens, err
}

// Delete removes the token with the given ID if it belongs to ownerSub. It
// returns ErrNotFound otherwise, so one user can't probe another's token IDs.
func (st *APITokenStore) Delete(ctx context.Context, ownerSub string, id uuid.UUID) error {
	return requireOneRow(st.db.WithContext(ctx).Delete(&APIToken{}, "id = ? AND owner_sub = ?", id, ownerSub))
}

// Authenticate returns the stored token matching secret, or
// ErrInvalidAPIToken if there is none or it has expired. It records the use
// at most once per lastUsedGranularity.
func (st *APITokenStore) Authenticate(ctx context.Context, secret string) (APIToken, error) {
	if !strings.HasPrefix(secret, APITokenPrefix) {
		return APIToken{}, ErrInvalidAPIToken
	}

	var t APIToken

	err := st.db.WithContext(ctx).Take(&t, "token_hash = ?", hashAPIToken(secret)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return APIToken{}, ErrInvalidAPIToken
	}

	if err != nil {
		return APIToken{}, err
	}

	now := time.Now().UTC()
	if t.Expired(now) {
		return APIToken{}, ErrInvalidAPIToken
	}

	if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) >= lastUsedGranularity {
		// Bookkeeping only: a failed write must not reject a valid token.
		if err := st.db.WithContext(ctx).Model(&t).Update("last_used_at", now).Error; err == nil {
			t.LastUsedAt = &now
		}
	}

	return t, nil
}

func newAPITokenSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return APITokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// hashAPIToken hashes a token secret for storage and lookup. The secret has
// 256 bits of entropy, so a fast unsalted hash is enough: there is nothing to
// brute-force.
func hashAPIToken(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}
