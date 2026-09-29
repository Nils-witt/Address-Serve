package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/nils-witt/address-serve/internal/store"
)

func registerStreetRoutes(mux *http.ServeMux, streets *store.StreetStore) {
	mux.HandleFunc("POST /api/streets", handleCreateStreet(streets))
	mux.HandleFunc("GET /api/streets", handleListStreets(streets))
	mux.HandleFunc("GET /api/cities", handleListCities(streets))
	mux.HandleFunc("GET /api/districts", handleListDistricts(streets))
	mux.HandleFunc("GET /api/streets/{id}", handleGetStreet(streets))
	mux.HandleFunc("PUT /api/streets/{id}", handleUpdateStreet(streets))
	mux.HandleFunc("DELETE /api/streets/{id}", handleDeleteStreet(streets))
}

//nolint:dupl // mirrors handleCreateHouseNumber; distinct entity types make a shared generic handler less readable than the duplication
func handleCreateStreet(streets *store.StreetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var s store.Street
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := s.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		created, err := streets.Create(r.Context(), s)
		if errors.Is(err, store.ErrStreetAlreadyExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, created)
	}
}

func handleListStreets(streets *store.StreetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		filter := store.StreetFilter{
			City:     query.Get("city"),
			District: query.Get("district"),
			Name:     query.Get("name"),
			Country:  query.Get("country"),
		}

		list, err := streets.List(r.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, list)
	}
}

func handleListCities(streets *store.StreetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cities, err := streets.ListCities(r.Context(), r.URL.Query().Get("name"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, cities)
	}
}

func handleListDistricts(streets *store.StreetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		filter := store.DistrictFilter{
			Name: query.Get("name"),
			City: query.Get("city"),
		}

		districts, err := streets.ListDistricts(r.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, districts)
	}
}

//nolint:dupl // mirrors the get/update/delete house-number handlers; distinct entity types make a shared generic handler less readable than the duplication
func handleGetStreet(streets *store.StreetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		s, err := streets.Get(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "street not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, s)
	}
}

func handleUpdateStreet(streets *store.StreetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		var s store.Street
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := s.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		updated, err := streets.Update(r.Context(), id, s)
		if errors.Is(err, store.ErrStreetAlreadyExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "street not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, updated)
	}
}

func handleDeleteStreet(streets *store.StreetStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		err = streets.Delete(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "street not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
