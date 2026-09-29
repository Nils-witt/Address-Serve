package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/nils-witt/address-serve/internal/store"
)

func registerBulkRoutes(mux *http.ServeMux, bulk *store.BulkStore) {
	mux.HandleFunc("POST /api/bulk", func(w http.ResponseWriter, r *http.Request) {
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

		results, err := bulk.Create(r.Context(), inputs)
		if errors.Is(err, store.ErrStreetAlreadyExists) {
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
