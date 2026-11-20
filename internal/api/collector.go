package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/store"
)

// collectorRunHeader names the collector run a write is made in. Collector
// tokens must send it on every write; other principals must not.
const collectorRunHeader = "X-Collector-Run"

// runRefFromRequest returns the collector run that r writes in, or nil if r
// is not a collector's. The error is meant for a 400 response.
func runRefFromRequest(r *http.Request) (*store.RunRef, error) {
	p, _ := principalFrom(r.Context())
	header := r.Header.Get(collectorRunHeader)

	switch {
	case p.CollectorID == nil && header == "":
		return nil, nil //nolint:nilnil // a write outside any run is not an error and has no run
	case p.CollectorID == nil:
		return nil, errors.New(collectorRunHeader + " can only be sent with a collector token")
	case header == "":
		return nil, errors.New("collector writes must name their run in the " + collectorRunHeader + " header")
	}

	runID, err := uuid.Parse(header)
	if err != nil {
		return nil, errors.New(collectorRunHeader + " must be a run id")
	}

	return &store.RunRef{CollectorID: *p.CollectorID, RunID: runID}, nil
}

// writeRunWriteError writes the response for an error of a write in a
// collector run, and reports whether err was one.
func writeRunWriteError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, store.ErrRunNotFound):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrRunNotRunning):
		writeError(w, http.StatusConflict, err.Error())
	default:
		return false
	}

	return true
}
