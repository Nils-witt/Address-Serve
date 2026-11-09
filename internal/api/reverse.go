package api

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"

	"github.com/nils-witt/address-serve/internal/store"
)

// maxReverseLimit caps the limit query parameter of GET /api/reverse.
const maxReverseLimit = 100

func registerReverseRoutes(mux *http.ServeMux, houseNumbers *store.HouseNumberStore, geoObjects *store.GeoObjectStore) {
	mux.HandleFunc("GET /api/reverse", handleReverseGeocode(houseNumbers, geoObjects))
}

// reverseResult is the response of GET /api/reverse. Each list is ranked by
// its own distance from the query point.
type reverseResult struct {
	HouseNumbers []store.NearbyHouseNumber `json:"houseNumbers"`
	GeoObjects   []store.NearbyGeoObject   `json:"geoObjects"`
}

// handleReverseGeocode serves GET /api/reverse: the house numbers and the geo
// objects nearest to the lat/lon query parameters, each nearest first. limit
// (default 1) sets how many of each are returned.
func handleReverseGeocode(houseNumbers *store.HouseNumberStore, geoObjects *store.GeoObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		lat, err := coordinateParam(query, "lat", 90)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		lon, err := coordinateParam(query, "lon", 180)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		limit := 1
		if v := query.Get("limit"); v != "" {
			limit, err = strconv.Atoi(v)
			if err != nil || limit < 1 || limit > maxReverseLimit {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("limit must be an integer between 1 and %d", maxReverseLimit))
				return
			}
		}

		nearbyHouseNumbers, err := houseNumbers.Nearest(r.Context(), lat, lon, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		nearbyGeoObjects, err := geoObjects.Nearest(r.Context(), lat, lon, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, reverseResult{HouseNumbers: nearbyHouseNumbers, GeoObjects: nearbyGeoObjects})
	}
}

// coordinateParam parses the required query parameter name as a number
// between -bound and bound.
func coordinateParam(query url.Values, name string, bound float64) (float64, error) {
	v := query.Get(name)
	if v == "" {
		return 0, fmt.Errorf("%s is required", name)
	}

	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || f < -bound || f > bound {
		return 0, fmt.Errorf("%s must be a number between %g and %g", name, -bound, bound)
	}

	return f, nil
}
