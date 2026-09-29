package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// oidcSigningMethods are the asymmetric algorithms accepted for tokens issued
// by the OpenID provider. HMAC is deliberately excluded so a token can never be
// verified against a public key used as an HMAC secret.
var oidcSigningMethods = []string{
	"RS256", "RS384", "RS512",
	"PS256", "PS384", "PS512",
	"ES256", "ES384", "ES512",
	"EdDSA",
}

// OIDCConfig configures verification of JWTs issued by an OpenID Connect
// provider.
type OIDCConfig struct {
	// IssuerURL is the provider's issuer identifier. It must match the "iss"
	// claim of accepted tokens exactly.
	IssuerURL string
	// Audience must be contained in the "aud" claim of accepted tokens,
	// usually the client ID registered for this API.
	Audience string
	// JWKSURL overrides the key set URL found via OpenID discovery.
	JWKSURL string
}

// OIDCVerifier verifies access and ID tokens against the signing keys
// published by an OpenID Connect provider.
type OIDCVerifier struct {
	keys   keyfunc.Keyfunc
	parser *jwt.Parser
}

// NewOIDCVerifier resolves the provider's JWKS endpoint (via discovery unless
// cfg.JWKSURL is set) and keeps its key set refreshed in the background until
// ctx is cancelled.
func NewOIDCVerifier(ctx context.Context, cfg OIDCConfig) (*OIDCVerifier, error) {
	if cfg.IssuerURL == "" {
		return nil, errors.New("OIDC issuer URL must be set")
	}

	if cfg.Audience == "" {
		return nil, errors.New("OIDC audience must be set")
	}

	jwksURL := cfg.JWKSURL
	if jwksURL == "" {
		var err error

		jwksURL, err = discoverJWKSURL(ctx, cfg.IssuerURL)
		if err != nil {
			return nil, err
		}
	}

	keys, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("failed to load OIDC key set from %s: %w", jwksURL, err)
	}

	return &OIDCVerifier{
		keys: keys,
		parser: jwt.NewParser(
			jwt.WithValidMethods(oidcSigningMethods),
			jwt.WithIssuer(cfg.IssuerURL),
			jwt.WithAudience(cfg.Audience),
			jwt.WithExpirationRequired(),
			jwt.WithLeeway(30*time.Second),
		),
	}, nil
}

func (v *OIDCVerifier) verify(tokenString string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}

	token, err := v.parser.ParseWithClaims(tokenString, claims, v.keys.Keyfunc)
	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("token is not valid")
	}

	return claims, nil
}

// discoverJWKSURL fetches the provider's OpenID configuration document and
// returns its jwks_uri.
func discoverJWKSURL(ctx context.Context, issuer string) (string, error) {
	discoveryURL := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build OIDC discovery request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch OIDC discovery document: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC discovery document at %s returned status %d", discoveryURL, resp.StatusCode)
	}

	var doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", fmt.Errorf("failed to decode OIDC discovery document: %w", err)
	}

	if doc.Issuer != issuer {
		return "", fmt.Errorf("OIDC discovery issuer %q does not match configured issuer %q", doc.Issuer, issuer)
	}

	if doc.JWKSURI == "" {
		return "", errors.New("OIDC discovery document has no jwks_uri")
	}

	return doc.JWKSURI, nil
}
