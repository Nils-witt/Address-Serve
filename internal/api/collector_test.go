package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/store"
)

func TestRunRefFromRequest(t *testing.T) {
	t.Parallel()

	collectorID, runID := uuid.New(), uuid.New()
	collector := principal{Subject: testSubject, APITokenID: &runID, CollectorID: &collectorID}
	user := principal{Subject: testSubject}

	tests := []struct {
		name    string
		who     principal
		header  string
		wantRun bool
		wantErr bool
	}{
		{name: "user without run", who: user},
		{name: "user naming a run", who: user, header: runID.String(), wantErr: true},
		{name: "collector naming its run", who: collector, header: runID.String(), wantRun: true},
		{name: "collector without run", who: collector, wantErr: true},
		{name: "collector with malformed run", who: collector, header: "nope", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.WithValue(t.Context(), principalContextKey, tt.who)
			req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/bulk", nil)

			if tt.header != "" {
				req.Header.Set(collectorRunHeader, tt.header)
			}

			run, err := runRefFromRequest(req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}

			if (run != nil) != tt.wantRun {
				t.Fatalf("run = %+v, wantRun %v", run, tt.wantRun)
			}

			if run != nil && (run.CollectorID != collectorID || run.RunID != runID) {
				t.Fatalf("run = %+v, want collector %s run %s", run, collectorID, runID)
			}
		})
	}
}

// collectorFlowClient sends requests to the full handler in
// TestCollectorFlow.
type collectorFlowClient struct {
	t       *testing.T
	handler http.Handler
}

// do sends a request with the given bearer token and, if run is set, the
// collector run header. It fails the test unless the status is want, and
// decodes the response into out if it is non-nil.
func (c collectorFlowClient) do(method, path, bearer, run, body string, want int, out any) {
	c.t.Helper()

	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}

	req := httptest.NewRequestWithContext(c.t.Context(), method, path, r)
	req.Header.Set("Authorization", "Bearer "+bearer)

	if run != "" {
		req.Header.Set(collectorRunHeader, run)
	}

	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	if rec.Code != want {
		c.t.Fatalf("%s %s: status = %d, want %d (body %s)", method, path, rec.Code, want, rec.Body)
	}

	if out != nil {
		if err := json.NewDecoder(rec.Body).Decode(out); err != nil {
			c.t.Fatalf("%s %s: decode response: %v", method, path, err)
		}
	}
}

// TestCollectorFlow registers a collector as an OIDC user and runs it twice
// with its token, through the full handler against a real database.
func TestCollectorFlow(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	oidc, sign := newTestOIDC(t)
	handler := NewHandler(Config{OIDC: oidc}, store.NewStreetStore(db), store.NewHouseNumberStore(db), store.NewGeoObjectStore(db),
		store.NewBulkStore(db), store.NewAPITokenStore(db), store.NewCollectorStore(db))
	c := collectorFlowClient{t: t, handler: handler}
	user := sign("collector-flow-" + uuid.NewString())
	tag := uuid.NewString()

	var created createdCollector

	c.do(http.MethodPost, "/api/collectors", user, "", `{"name":"flow `+tag+`","expectedIntervalMinutes":60}`, http.StatusCreated, &created)
	t.Cleanup(func() { _ = store.NewCollectorStore(db).Delete(context.Background(), created.ID) })

	collector := created.Token
	c.do(http.MethodGet, "/api/collectors", collector, "", "", http.StatusForbidden, nil)
	c.do(http.MethodPost, "/api/collector/runs", user, "", `{"mode":"full"}`, http.StatusForbidden, nil)
	c.do(http.MethodPost, "/api/collector/heartbeat", collector, "", "", http.StatusNoContent, nil)

	streets := func(numbers string) string {
		return fmt.Sprintf(`[{"city":"Flow","district":"D","name":"Flowweg %s","country":"DE","latitude":50,"longitude":7,"houseNumbers":[%s]}]`, tag, numbers)
	}
	number := func(n int) string {
		return fmt.Sprintf(`{"number":%d,"postcode":"53111","latitude":50,"longitude":7}`, n)
	}

	var run store.CollectorRun

	c.do(http.MethodPost, "/api/collector/runs", collector, "", `{"mode":"full"}`, http.StatusCreated, &run)
	c.do(http.MethodPost, "/api/bulk", collector, "", streets(number(1)), http.StatusBadRequest, nil)
	c.do(http.MethodPost, "/api/bulk", collector, run.ID.String(), streets(number(1)+","+number(2)), http.StatusOK, nil)

	var results []store.BulkResult

	c.do(http.MethodPost, "/api/bulk", collector, run.ID.String(), streets(number(1)+","+number(2)), http.StatusOK, &results)
	c.t.Cleanup(func() { db.Delete(&store.Street{}, "id = ?", results[0].Street.ID) })

	geo := fmt.Sprintf(`{"name":"flow %s","group":"flow","streetId":"%s","latitude":50,"longitude":7}`, tag, results[0].Street.ID)
	c.do(http.MethodPut, "/api/geo-objects/by-external-id/flow:"+tag, collector, run.ID.String(), geo, http.StatusCreated, nil)
	c.do(http.MethodPost, "/api/collector/runs/"+run.ID.String()+"/finish", collector, "", `{"state":"succeeded"}`, http.StatusOK, &run)

	if run.Streets != 2 || run.HouseNumbers != 4 || run.GeoObjects != 1 || run.Deleted != 0 {
		t.Fatalf("first run = %+v, want 2 streets, 4 house numbers, 1 geo object, nothing deleted", run)
	}

	// The second full run no longer sends house number 2 or the geo object.
	c.do(http.MethodPost, "/api/collector/runs", collector, "", `{"mode":"full"}`, http.StatusCreated, &run)
	c.do(http.MethodPost, "/api/bulk", collector, run.ID.String(), streets(number(1)), http.StatusOK, nil)
	c.do(http.MethodPost, "/api/collector/runs/"+run.ID.String()+"/finish", collector, "", `{"state":"succeeded"}`, http.StatusOK, &run)
	c.do(http.MethodPost, "/api/collector/runs/"+run.ID.String()+"/finish", collector, "", `{"state":"succeeded"}`, http.StatusConflict, nil)

	if run.Deleted != 2 {
		t.Fatalf("second run deleted %d rows, want house number 2 and the geo object", run.Deleted)
	}

	var got store.Collector

	c.do(http.MethodGet, "/api/collectors/"+created.ID.String(), user, "", "", http.StatusOK, &got)

	if got.Status != store.CollectorStatusOK || got.LastRun == nil || got.LastRun.ID != run.ID {
		t.Fatalf("collector = %+v, want ok with the second run last", got)
	}

	var runs []store.CollectorRun

	c.do(http.MethodGet, "/api/collectors/"+created.ID.String()+"/runs?limit=1", user, "", "", http.StatusOK, &runs)

	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs = %+v, want only the second run", runs)
	}

	var rotated rotatedCollectorToken

	c.do(http.MethodPost, "/api/collectors/"+created.ID.String()+"/token", user, "", "", http.StatusOK, &rotated)
	c.do(http.MethodGet, "/api/collector", collector, "", "", http.StatusUnauthorized, nil)
	c.do(http.MethodGet, "/api/collector", rotated.Token, "", "", http.StatusOK, nil)
	c.do(http.MethodDelete, "/api/collectors/"+created.ID.String(), user, "", "", http.StatusNoContent, nil)
	c.do(http.MethodGet, "/api/collector", rotated.Token, "", "", http.StatusUnauthorized, nil)
}
