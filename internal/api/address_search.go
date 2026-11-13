package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/nils-witt/address-serve/internal/store"
)

const (
	defaultAddressSearchLimit = 10
	maxAddressSearchLimit     = 50
	minAddressSearchLength    = 2
)

func registerAddressSearchRoutes(mux *http.ServeMux, houseNumbers *store.HouseNumberStore) {
	mux.HandleFunc("GET /api/addresses/search", handleSearchAddresses(houseNumbers))
}

// parseAddressSearchLimit parses the limit query parameter, defaulting to
// defaultAddressSearchLimit when it is empty.
func parseAddressSearchLimit(v string) (int, bool) {
	if v == "" {
		return defaultAddressSearchLimit, true
	}

	limit, err := strconv.Atoi(v)
	if err != nil || limit < 1 || limit > maxAddressSearchLimit {
		return 0, false
	}

	return limit, true
}

func handleSearchAddresses(houseNumbers *store.HouseNumberStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		q := store.ParseAddressQuery(query.Get("q"))
		if len([]rune(strings.TrimSpace(q.Street))) < minAddressSearchLength {
			writeError(w, http.StatusBadRequest, "q must name at least 2 characters of a street")
			return
		}

		limit, ok := parseAddressSearchLimit(query.Get("limit"))
		if !ok {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 50")
			return
		}

		matches, err := houseNumbers.SearchAddresses(r.Context(), q, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, matches)
	}
}
