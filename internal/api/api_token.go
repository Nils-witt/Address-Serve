package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nils-witt/address-serve/internal/store"
)

const maxAPITokenNameLen = 100

func registerAPITokenRoutes(mux *http.ServeMux, tokens *store.APITokenStore) {
	mux.HandleFunc("GET /api/tokens", handleListAPITokens(tokens))
	mux.HandleFunc("POST /api/tokens", handleCreateAPIToken(tokens))
	mux.HandleFunc("DELETE /api/tokens/{id}", handleDeleteAPIToken(tokens))
}

// requireTokenOwner returns the principal allowed to manage API tokens, or
// writes a 403 and returns false. Tokens are managed only from an OIDC
// session, so a leaked API token can't be used to mint more of them.
func requireTokenOwner(w http.ResponseWriter, r *http.Request) (principal, bool) {
	p, ok := principalFrom(r.Context())
	if !ok || p.APITokenID != nil || p.Subject == "" {
		writeError(w, http.StatusForbidden, "API tokens can only be managed when signed in with OpenID Connect")
		return principal{}, false
	}

	return p, true
}

func handleListAPITokens(tokens *store.APITokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := requireTokenOwner(w, r)
		if !ok {
			return
		}

		list, err := tokens.List(r.Context(), p.Subject)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, list)
	}
}

type createAPITokenRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

func (req *createAPITokenRequest) validate(now time.Time) error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return errors.New("name is required")
	}

	if utf8.RuneCountInString(req.Name) > maxAPITokenNameLen {
		return errors.New("name must be at most 100 characters")
	}

	if req.ExpiresAt != nil && !req.ExpiresAt.After(now) {
		return errors.New("expiresAt must be in the future")
	}

	return nil
}

// createdAPIToken is the one response that carries the token secret.
type createdAPIToken struct {
	store.APIToken

	Token string `json:"token"`
}

func handleCreateAPIToken(tokens *store.APITokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := requireTokenOwner(w, r)
		if !ok {
			return
		}

		var req createAPITokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := req.validate(time.Now()); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		created, secret, err := tokens.Create(r.Context(), p.Subject, p.Name, req.Name, req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, createdAPIToken{APIToken: created, Token: secret})
	}
}

func handleDeleteAPIToken(tokens *store.APITokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := requireTokenOwner(w, r)
		if !ok {
			return
		}

		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		err = tokens.Delete(r.Context(), p.Subject, id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "API token not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
