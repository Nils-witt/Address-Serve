package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/store"
)

func registerGeoObjectRoutes(mux *http.ServeMux, geoObjects *store.GeoObjectStore) {
	mux.HandleFunc("POST /api/geo-objects", handleCreateGeoObject(geoObjects))
	mux.HandleFunc("GET /api/geo-objects", handleListGeoObjects(geoObjects))
	mux.HandleFunc("GET /api/geo-objects/{id}", handleGetGeoObject(geoObjects))
	mux.HandleFunc("PUT /api/geo-objects/{id}", handleUpdateGeoObject(geoObjects))
	mux.HandleFunc("DELETE /api/geo-objects/{id}", handleDeleteGeoObject(geoObjects))
}

// isGeoObjectReferenceError reports whether err means the geo object's street
// or house number reference is invalid, which is the client's fault.
func isGeoObjectReferenceError(err error) bool {
	return errors.Is(err, store.ErrStreetNotFound) ||
		errors.Is(err, store.ErrHouseNumberNotFound) ||
		errors.Is(err, store.ErrHouseNumberNotOnStreet)
}

func handleCreateGeoObject(geoObjects *store.GeoObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var g store.GeoObject
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := g.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		created, err := geoObjects.Create(r.Context(), g)
		if isGeoObjectReferenceError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, created)
	}
}

func handleListGeoObjects(geoObjects *store.GeoObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		filter := store.GeoObjectFilter{
			Name:  query.Get("name"),
			Group: query.Get("group"),
		}

		var err error

		if filter.StreetID, err = optionalUUIDParam(query, "streetId"); err != nil {
			writeError(w, http.StatusBadRequest, "invalid streetId")
			return
		}

		if filter.HouseNumberID, err = optionalUUIDParam(query, "houseNumberId"); err != nil {
			writeError(w, http.StatusBadRequest, "invalid houseNumberId")
			return
		}

		list, err := geoObjects.List(r.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, list)
	}
}

//nolint:dupl // mirrors the get/update/delete house-number handlers; distinct entity types make a shared generic handler less readable than the duplication
func handleGetGeoObject(geoObjects *store.GeoObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		g, err := geoObjects.Get(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "geo object not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, g)
	}
}

func handleUpdateGeoObject(geoObjects *store.GeoObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		var g store.GeoObject
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := g.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		updated, err := geoObjects.Update(r.Context(), id, g)
		if isGeoObjectReferenceError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "geo object not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, updated)
	}
}

//nolint:dupl // mirrors handleDeleteStreet and handleDeleteHouseNumber; distinct entity types make a shared generic handler less readable than the duplication
func handleDeleteGeoObject(geoObjects *store.GeoObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		err = geoObjects.Delete(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "geo object not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// optionalUUIDParam parses the query parameter key as a UUID, returning nil
// if it is absent or empty.
func optionalUUIDParam(query url.Values, key string) (*uuid.UUID, error) {
	v := query.Get(key)
	if v == "" {
		return nil, nil //nolint:nilnil // an absent parameter is not an error and has no value
	}

	id, err := uuid.Parse(v)
	if err != nil {
		return nil, err
	}

	return &id, nil
}
