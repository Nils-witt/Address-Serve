package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/store"
)

func registerHouseNumberRoutes(mux *http.ServeMux, houseNumbers *store.HouseNumberStore) {
	mux.HandleFunc("POST /api/house-numbers", handleCreateHouseNumber(houseNumbers))
	mux.HandleFunc("GET /api/house-numbers", handleListHouseNumbers(houseNumbers))
	mux.HandleFunc("GET /api/house-numbers/{id}", handleGetHouseNumber(houseNumbers))
	mux.HandleFunc("PUT /api/house-numbers/{id}", handleUpdateHouseNumber(houseNumbers))
	mux.HandleFunc("DELETE /api/house-numbers/{id}", handleDeleteHouseNumber(houseNumbers))
}

//nolint:dupl // mirrors handleCreateStreet; distinct entity types make a shared generic handler less readable than the duplication
func handleCreateHouseNumber(houseNumbers *store.HouseNumberStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var h store.HouseNumber
		if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := h.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		created, err := houseNumbers.Create(r.Context(), h)
		if errors.Is(err, store.ErrStreetNotFound) {
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

func handleListHouseNumbers(houseNumbers *store.HouseNumberStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		var filter store.HouseNumberFilter

		if v := query.Get("streetId"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid streetId")
				return
			}

			filter.StreetID = &id
		}

		if v := query.Get("number"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid number")
				return
			}

			filter.Number = &n
		}

		list, err := houseNumbers.List(r.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, list)
	}
}

//nolint:dupl // mirrors the get/update/delete street handlers; distinct entity types make a shared generic handler less readable than the duplication
func handleGetHouseNumber(houseNumbers *store.HouseNumberStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		h, err := houseNumbers.Get(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "house number not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, h)
	}
}

func handleUpdateHouseNumber(houseNumbers *store.HouseNumberStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		var h store.HouseNumber
		if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if err := h.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		updated, err := houseNumbers.Update(r.Context(), id, h)
		if errors.Is(err, store.ErrStreetNotFound) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "house number not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, updated)
	}
}

func handleDeleteHouseNumber(houseNumbers *store.HouseNumberStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		err = houseNumbers.Delete(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "house number not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
