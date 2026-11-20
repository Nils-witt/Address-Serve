package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/nils-witt/address-serve/internal/store"
)

// registerBulkRoutes serves POST /api/bulk. Without a collector run it
// creates every street, failing with 409 if one exists; in a collector run
// (see runRefFromRequest) it upserts them instead, answering 200.
func registerBulkRoutes(mux *http.ServeMux, bulk *store.BulkStore) {
	mux.HandleFunc("POST /api/bulk", func(w http.ResponseWriter, r *http.Request) {
		run, err := runRefFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		var inputs []store.BulkStreetInput
		if err := json.NewDecoder(r.Body).Decode(&inputs); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if len(inputs) == 0 {
			writeError(w, http.StatusBadRequest, "at least one street is required")
			return
		}

		for i, input := range inputs {
			if err := input.Validate(); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("streets[%d]: %s", i, err.Error()))
				return
			}
		}

		if run != nil {
			upsertBulk(w, r, bulk, *run, inputs)
			return
		}

		results, err := bulk.Create(r.Context(), inputs)
		if errors.Is(err, store.ErrStreetAlreadyExists) || errors.Is(err, store.ErrHouseNumberAlreadyExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, results)
	})
}

func upsertBulk(w http.ResponseWriter, r *http.Request, bulk *store.BulkStore, run store.RunRef, inputs []store.BulkStreetInput) {
	results, err := bulk.Upsert(r.Context(), run, inputs)
	if errors.Is(err, store.ErrStreetRepeated) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if writeRunWriteError(w, err) {
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, results)
}
