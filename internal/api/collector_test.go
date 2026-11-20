package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
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
