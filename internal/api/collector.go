package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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

const (
	defaultRunsLimit = 20
	maxRunsLimit     = 50
)

func registerCollectorRoutes(mux *http.ServeMux, collectors *store.CollectorStore) {
	// Managing collectors, for users. Collector tokens are kept out by
	// collectorMayCall.
	mux.HandleFunc("GET /api/collectors", handleListCollectors(collectors))
	mux.HandleFunc("POST /api/collectors", handleCreateCollector(collectors))
	mux.HandleFunc("GET /api/collectors/{id}", handleGetCollector(collectors))
	mux.HandleFunc("PUT /api/collectors/{id}", handleUpdateCollector(collectors))
	mux.HandleFunc("DELETE /api/collectors/{id}", handleDeleteCollector(collectors))
	mux.HandleFunc("POST /api/collectors/{id}/token", handleRotateCollectorToken(collectors))
	mux.HandleFunc("GET /api/collectors/{id}/runs", handleListCollectorRuns(collectors))

	// Used by collectors themselves, with their token.
	mux.HandleFunc("GET /api/collector", handleGetOwnCollector(collectors))
	mux.HandleFunc("POST /api/collector/heartbeat", handleCollectorHeartbeat(collectors))
	mux.HandleFunc("POST /api/collector/runs", handleStartCollectorRun(collectors))
	mux.HandleFunc("POST /api/collector/runs/{runId}/finish", handleFinishCollectorRun(collectors))
}

// writeCollectorError writes the response for an error of a collector
// store call.
func writeCollectorError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "collector not found")
	case errors.Is(err, store.ErrCollectorNameTaken):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// decodeCollectorInput reads and validates a CollectorInput body, or writes
// a 400 and returns false.
func decodeCollectorInput(w http.ResponseWriter, r *http.Request) (store.CollectorInput, bool) {
	var in store.CollectorInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return store.CollectorInput{}, false
	}

	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return store.CollectorInput{}, false
	}

	return in, true
}

func handleListCollectors(collectors *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := collectors.List(r.Context())
		if err != nil {
			writeCollectorError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, list)
	}
}

// createdCollector is the response to creating a collector, the one that
// carries its token secret.
type createdCollector struct {
	store.Collector

	Token string `json:"token"`
}

// handleCreateCollector creates a collector and its token. Like API tokens,
// collectors are managed only from an OIDC session (see requireTokenOwner),
// so a leaked API token can't mint collector tokens.
func handleCreateCollector(collectors *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := requireTokenOwner(w, r)
		if !ok {
			return
		}

		in, ok := decodeCollectorInput(w, r)
		if !ok {
			return
		}

		c, secret, err := collectors.Create(r.Context(), p.Subject, p.Name, in)
		if err != nil {
			writeCollectorError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, createdCollector{Collector: c, Token: secret})
	}
}

func handleGetCollector(collectors *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		c, err := collectors.Get(r.Context(), id)
		if err != nil {
			writeCollectorError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, c)
	}
}

func handleUpdateCollector(collectors *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireTokenOwner(w, r); !ok {
			return
		}

		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		in, ok := decodeCollectorInput(w, r)
		if !ok {
			return
		}

		c, err := collectors.Update(r.Context(), id, in)
		if err != nil {
			writeCollectorError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, c)
	}
}

func handleDeleteCollector(collectors *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireTokenOwner(w, r); !ok {
			return
		}

		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		if err := collectors.Delete(r.Context(), id); err != nil {
			writeCollectorError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// rotatedCollectorToken is the response to rotating a collector's token.
type rotatedCollectorToken struct {
	Token string `json:"token"`
}

func handleRotateCollectorToken(collectors *store.CollectorStore) http.HandlerFunc {
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

		secret, err := collectors.RotateToken(r.Context(), id, p.Subject, p.Name)
		if err != nil {
			writeCollectorError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, rotatedCollectorToken{Token: secret})
	}
}

func handleListCollectorRuns(collectors *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := idFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		limit := defaultRunsLimit

		if v := r.URL.Query().Get("limit"); v != "" {
			limit, err = strconv.Atoi(v)
			if err != nil || limit < 1 || limit > maxRunsLimit {
				writeError(w, http.StatusBadRequest, "limit must be between 1 and 50")
				return
			}
		}

		runs, err := collectors.ListRuns(r.Context(), id, limit)
		if err != nil {
			writeCollectorError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, runs)
	}
}

// requireCollector returns the ID of the collector whose token made r, or
// writes a 403 and returns false.
func requireCollector(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	p, _ := principalFrom(r.Context())
	if p.CollectorID == nil {
		writeError(w, http.StatusForbidden, "only collector tokens can call this route")
		return uuid.Nil, false
	}

	return *p.CollectorID, true
}

func handleGetOwnCollector(collectors *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := requireCollector(w, r)
		if !ok {
			return
		}

		c, err := collectors.Get(r.Context(), id)
		if err != nil {
			writeCollectorError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, c)
	}
}

func handleCollectorHeartbeat(collectors *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := requireCollector(w, r)
		if !ok {
			return
		}

		if err := collectors.Touch(r.Context(), id); err != nil {
			writeCollectorError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

type startRunRequest struct {
	Mode store.RunMode `json:"mode"`
}

func handleStartCollectorRun(collectors *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := requireCollector(w, r)
		if !ok {
			return
		}

		var req startRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if !req.Mode.Valid() {
			writeError(w, http.StatusBadRequest, `mode must be "incremental" or "full"`)
			return
		}

		run, err := collectors.StartRun(r.Context(), id, req.Mode)
		if errors.Is(err, store.ErrRunInProgress) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}

		if err != nil {
			writeCollectorError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, run)
	}
}

type finishRunRequest struct {
	State store.RunState `json:"state"`
	Error string         `json:"error"`
}

func handleFinishCollectorRun(collectors *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := requireCollector(w, r)
		if !ok {
			return
		}

		runID, err := uuid.Parse(r.PathValue("runId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid run id")
			return
		}

		var req finishRunRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if !req.State.ValidFinal() {
			writeError(w, http.StatusBadRequest, `state must be "succeeded" or "failed"`)
			return
		}

		run, err := collectors.FinishRun(r.Context(), id, runID, req.State, req.Error)

		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "run not found")
		case errors.Is(err, store.ErrRunNotRunning):
			writeError(w, http.StatusConflict, err.Error())
		case err != nil:
			writeError(w, http.StatusInternalServerError, err.Error())
		default:
			writeJSON(w, http.StatusOK, run)
		}
	}
}
