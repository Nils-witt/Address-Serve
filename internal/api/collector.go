package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/collector"
	"github.com/nils-witt/address-serve/internal/collector/contract"
	"github.com/nils-witt/address-serve/internal/store"
)

// CollectorConfig wires the collector routes. A nil Runner disables them.
type CollectorConfig struct {
	Runner *collector.Runner
	Store  *store.CollectorStore
	Admin  AdminClaim
}

const (
	defaultRunsLimit = 20
	maxRunsLimit     = 100
	// reloadWriteTimeout gives a reload time to describe every collector,
	// beyond the server's default write timeout.
	reloadWriteTimeout = collector.DescribeTimeout + 10*time.Second
)

func registerCollectorRoutes(mux *http.ServeMux, cfg CollectorConfig) {
	routes := map[string]http.HandlerFunc{
		"GET /api/collectors":                     handleListCollectors(cfg.Store),
		"POST /api/collectors/reload":             handleReloadCollectors(cfg),
		"GET /api/collectors/{id}":                handleGetCollector(cfg.Store),
		"PUT /api/collectors/{id}/settings":       handleUpdateCollectorSettings(cfg.Store),
		"DELETE /api/collectors/{id}":             handleDeleteCollector(cfg.Store),
		"POST /api/collectors/{id}/runs":          handleStartRun(cfg.Runner),
		"GET /api/collectors/{id}/runs":           handleListRuns(cfg.Store),
		"GET /api/collector-runs/{runId}":         handleGetRun(cfg.Store),
		"POST /api/collector-runs/{runId}/cancel": handleCancelRun(cfg),
	}

	for pattern, h := range routes {
		mux.Handle(pattern, requireAdmin(cfg.Admin, h))
	}
}

// collectorView is a collector as the API shows it: settings without the
// values of secrets, which only say whether they are set.
type collectorView struct {
	store.Collector

	Settings   map[string]json.RawMessage `json:"settings"`
	SecretsSet []string                   `json:"secretsSet"`
	LastRun    *store.CollectorRun        `json:"lastRun,omitempty"`
}

func newCollectorView(c store.Collector, lastRun *store.CollectorRun) collectorView {
	v := collectorView{
		Collector: c, Settings: map[string]json.RawMessage{}, SecretsSet: []string{}, LastRun: lastRun,
	}

	for _, spec := range c.SettingsSchema {
		value := c.Settings[spec.Key]

		switch {
		case value == nil:
		case spec.Type == contract.SettingSecret:
			v.SecretsSet = append(v.SecretsSet, spec.Key)
		default:
			v.Settings[spec.Key] = value
		}
	}

	return v
}

func writeCollectorList(w http.ResponseWriter, r *http.Request, cs *store.CollectorStore, collectors []store.Collector) {
	lastRuns, err := cs.LastRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	views := make([]collectorView, len(collectors))

	for i, c := range collectors {
		var last *store.CollectorRun
		if run, ok := lastRuns[c.ID]; ok {
			last = &run
		}

		views[i] = newCollectorView(c, last)
	}

	writeJSON(w, http.StatusOK, views)
}

func handleListCollectors(cs *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		collectors, err := cs.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeCollectorList(w, r, cs, collectors)
	}
}

func handleReloadCollectors(cfg CollectorConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(reloadWriteTimeout))

		collectors, err := cfg.Runner.Reload(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeCollectorList(w, r, cfg.Store, collectors)
	}
}

// writeCollector writes the collector with the given ID, or a 404.
func writeCollector(w http.ResponseWriter, r *http.Request, cs *store.CollectorStore, c store.Collector, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "collector not found")
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	lastRuns, err := cs.LastRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var last *store.CollectorRun
	if run, ok := lastRuns[c.ID]; ok {
		last = &run
	}

	writeJSON(w, http.StatusOK, newCollectorView(c, last))
}

func handleGetCollector(cs *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := cs.Get(r.Context(), r.PathValue("id"))
		writeCollector(w, r, cs, c, err)
	}
}

func handleUpdateCollectorSettings(cs *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input store.SettingsInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input == nil {
			writeError(w, http.StatusBadRequest, "body must be a JSON object of settings")
			return
		}

		c, err := cs.UpdateSettings(r.Context(), r.PathValue("id"), input)
		if errors.Is(err, store.ErrInvalidSettings) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		writeCollector(w, r, cs, c, err)
	}
}

func handleDeleteCollector(cs *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deleteData, err := optionalBool(r, "deleteData")
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		err = cs.Delete(r.Context(), r.PathValue("id"), deleteData)

		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "collector not found")
		case errors.Is(err, store.ErrCollectorAvailable), errors.Is(err, store.ErrRunActive):
			writeError(w, http.StatusConflict, err.Error())
		case err != nil:
			writeError(w, http.StatusInternalServerError, err.Error())
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func handleStartRun(runner *collector.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, _ := principalFrom(r.Context())

		run, err := runner.Start(r.Context(), r.PathValue("id"), p.Name)

		var missing *collector.MissingSettingsError

		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "collector not found")
		case errors.As(err, &missing):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, collector.ErrUnavailable), errors.Is(err, store.ErrRunActive):
			writeError(w, http.StatusConflict, err.Error())
		case err != nil:
			writeError(w, http.StatusInternalServerError, err.Error())
		default:
			writeJSON(w, http.StatusAccepted, run)
		}
	}
}

func handleListRuns(cs *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset, err := pagination(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		id := r.PathValue("id")
		if _, err := cs.Get(r.Context(), id); errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "collector not found")
			return
		}

		runs, err := cs.ListRuns(r.Context(), id, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, runs)
	}
}

func handleGetRun(cs *store.CollectorStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("runId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid run id")
			return
		}

		run, err := cs.GetRun(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}

		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, run)
	}
}

func handleCancelRun(cfg CollectorConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.PathValue("runId"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid run id")
			return
		}

		if err := cfg.Runner.Cancel(id); errors.Is(err, collector.ErrRunNotActive) {
			if _, err := cfg.Store.GetRun(r.Context(), id); errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "run not found")
				return
			}

			writeError(w, http.StatusConflict, err.Error())

			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// pagination reads the limit and offset query parameters.
func pagination(r *http.Request) (limit, offset int, err error) {
	limit, offset = defaultRunsLimit, 0

	if s := r.URL.Query().Get("limit"); s != "" {
		limit, err = strconv.Atoi(s)
		if err != nil || limit < 1 || limit > maxRunsLimit {
			return 0, 0, errors.New("limit must be between 1 and 100")
		}
	}

	if s := r.URL.Query().Get("offset"); s != "" {
		offset, err = strconv.Atoi(s)
		if err != nil || offset < 0 {
			return 0, 0, errors.New("offset must be a non-negative integer")
		}
	}

	return limit, offset, nil
}

// optionalBool reads a query parameter that is absent, "true" or "false".
func optionalBool(r *http.Request, key string) (bool, error) {
	s := r.URL.Query().Get(key)
	if s == "" {
		return false, nil
	}

	b, err := strconv.ParseBool(s)
	if err != nil {
		return false, errors.New(key + " must be true or false")
	}

	return b, nil
}
