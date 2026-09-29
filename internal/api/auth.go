package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type contextKey string

const claimsContextKey contextKey = "claims"

var (
	errMissingAuthHeader   = errors.New("missing Authorization header")
	errMalformedAuthHeader = errors.New("authorization header must be in the form 'Bearer <token>'")
)

// authMiddleware requires a valid JWT bearer token for every request except
// GET, HEAD and OPTIONS, which stay open for read access. Tokens must be issued
// by the configured OpenID Connect provider.
func authMiddleware(verifier *OIDCVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			tokenString, err := bearerToken(r)
			if err != nil {
				writeError(w, http.StatusUnauthorized, err.Error())
				return
			}

			claims, err := verifier.verify(tokenString)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", errMissingAuthHeader
	}

	scheme, token, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", errMalformedAuthHeader
	}

	return token, nil
}
