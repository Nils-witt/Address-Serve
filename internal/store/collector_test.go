package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCollectorStatus(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time {
		at := now.Add(-d)
		return &at
	}
	hourly := 60

	tests := []struct {
		name      string
		collector Collector
		expected  CollectorStatus
	}{
		{
			name:      "no runs",
			collector: Collector{ExpectedIntervalMinutes: &hourly},
			expected:  CollectorStatusNever,
		},
		{
			name: "open run with recent contact",
			collector: Collector{
				LastSeenAt: ago(time.Minute),
				LastRun:    &CollectorRun{State: RunStateRunning, StartedAt: *ago(2 * time.Hour)},
			},
			expected: CollectorStatusRunning,
		},
		{
			name: "open run without contact is abandoned",
			collector: Collector{
				LastSeenAt: ago(RunAbandonTimeout + time.Minute),
				LastRun:    &CollectorRun{State: RunStateRunning, StartedAt: *ago(2 * time.Hour)},
			},
			expected: CollectorStatusFailed,
		},
		{
			name:      "last run failed",
			collector: Collector{LastRun: &CollectorRun{State: RunStateFailed}},
			expected:  CollectorStatusFailed,
		},
		{
			name: "last run succeeded, no interval",
			collector: Collector{
				LastSuccessAt: ago(1000 * time.Hour),
				LastRun:       &CollectorRun{State: RunStateSucceeded},
			},
			expected: CollectorStatusOK,
		},
		{
			name: "success within twice the interval",
			collector: Collector{
				ExpectedIntervalMinutes: &hourly,
				LastSuccessAt:           ago(119 * time.Minute),
				LastRun:                 &CollectorRun{State: RunStateSucceeded},
			},
			expected: CollectorStatusOK,
		},
		{
			name: "success older than twice the interval",
			collector: Collector{
				ExpectedIntervalMinutes: &hourly,
				LastSuccessAt:           ago(121 * time.Minute),
				LastRun:                 &CollectorRun{State: RunStateSucceeded},
			},
			expected: CollectorStatusStale,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := collectorStatus(tt.collector, now); got != tt.expected {
				t.Fatalf("collectorStatus = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCollectorInput_Validate(t *testing.T) {
	t.Parallel()

	const name = "importer"

	zero, day := 0, 24*60

	tests := []struct {
		name    string
		input   CollectorInput
		wantErr bool
	}{
		{name: "name only", input: CollectorInput{Name: name}},
		{name: "with interval", input: CollectorInput{Name: name, ExpectedIntervalMinutes: &day}},
		{name: "blank name", input: CollectorInput{Name: "  "}, wantErr: true},
		{name: "name too long", input: CollectorInput{Name: strings.Repeat("x", 101)}, wantErr: true},
		{name: "description too long", input: CollectorInput{Name: name, Description: strings.Repeat("x", 501)}, wantErr: true},
		{name: "zero interval", input: CollectorInput{Name: name, ExpectedIntervalMinutes: &zero}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := tt.input.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// must returns a check of a call's results: it fails t on err and returns v
// otherwise. Use it as must(call())(t).
func must[T any](v T, err error) func(t *testing.T) T {
	return func(t *testing.T) T {
		t.Helper()

		if err != nil {
			t.Fatal(err)
		}

		return v
	}
}

// wantErrorIs fails t unless err matches target.
func wantErrorIs(t *testing.T, what string, err, target error) {
	t.Helper()

	if !errors.Is(err, target) {
		t.Fatalf("%s: err = %v, want %v", what, err, target)
	}
}

// createCollector creates a collector with a unique name and deletes it when
// t ends.
func createCollector(t *testing.T, cs *CollectorStore) Collector {
	t.Helper()

	c, _, err := cs.Create(t.Context(), "owner", "Owner", CollectorInput{Name: "collector " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = cs.Delete(context.Background(), c.ID) })

	return c
}

func TestCollectorStore_CRUD(t *testing.T) {
	t.Parallel()

	cs := NewCollectorStore(openBenchDB(t))
	c := createCollector(t, cs)
	other := createCollector(t, cs)

	if c.Status != CollectorStatusNever {
		t.Fatalf("new collector status = %q, want never", c.Status)
	}

	_, _, err := cs.Create(t.Context(), "owner", "Owner", CollectorInput{Name: c.Name})
	wantErrorIs(t, "Create with taken name", err, ErrCollectorNameTaken)

	hourly := 60
	updated := must(cs.Update(t.Context(), c.ID, CollectorInput{Name: " " + c.Name + " renamed ", Description: "d", ExpectedIntervalMinutes: &hourly}))(t)

	if updated.Name != c.Name+" renamed" || updated.Description != "d" || updated.ExpectedIntervalMinutes == nil || *updated.ExpectedIntervalMinutes != 60 {
		t.Fatalf("Update = %+v, want trimmed name, description and interval set", updated)
	}

	_, err = cs.Update(t.Context(), other.ID, CollectorInput{Name: updated.Name})
	wantErrorIs(t, "Update to taken name", err, ErrCollectorNameTaken)

	_, err = cs.Update(t.Context(), uuid.New(), CollectorInput{Name: "missing " + uuid.NewString()})
	wantErrorIs(t, "Update of unknown collector", err, ErrNotFound)

	found := 0

	for _, l := range must(cs.List(t.Context()))(t) {
		if l.ID == c.ID || l.ID == other.ID {
			found++
		}
	}

	if found != 2 {
		t.Fatalf("List holds %d of the 2 created collectors", found)
	}

	wantErrorIs(t, "Delete", cs.Delete(t.Context(), c.ID), nil)

	_, err = cs.Get(t.Context(), c.ID)
	wantErrorIs(t, "Get after Delete", err, ErrNotFound)
}

func TestCollectorStore_StartAndFinishRun(t *testing.T) {
	t.Parallel()

	cs := NewCollectorStore(openBenchDB(t))
	c := createCollector(t, cs)
	run := must(cs.StartRun(t.Context(), c.ID, RunModeIncremental))(t)

	_, err := cs.StartRun(t.Context(), c.ID, RunModeIncremental)
	wantErrorIs(t, "second StartRun", err, ErrRunInProgress)

	got := must(cs.Get(t.Context(), c.ID))(t)
	if got.Status != CollectorStatusRunning || got.LastRun == nil || got.LastRun.ID != run.ID || got.LastSeenAt == nil {
		t.Fatalf("Get during run = %+v, want running with last run %s and contact recorded", got, run.ID)
	}

	finished := must(cs.FinishRun(t.Context(), c.ID, run.ID, RunStateFailed, strings.Repeat("é", 3000)))(t)
	if finished.State != RunStateFailed || finished.FinishedAt == nil || len([]rune(finished.Error)) != maxRunErrorLen {
		t.Fatalf("FinishRun = %+v, want failed, finished and error cut to %d runes", finished, maxRunErrorLen)
	}

	_, err = cs.FinishRun(t.Context(), c.ID, run.ID, RunStateSucceeded, "")
	wantErrorIs(t, "FinishRun twice", err, ErrRunNotRunning)

	other := createCollector(t, cs)
	_, err = cs.FinishRun(t.Context(), other.ID, run.ID, RunStateSucceeded, "")
	wantErrorIs(t, "FinishRun by another collector", err, ErrNotFound)

	_, err = cs.ListRuns(t.Context(), uuid.New(), 10)
	wantErrorIs(t, "ListRuns of unknown collector", err, ErrNotFound)
}

func TestCollectorStore_AbandonedRun(t *testing.T) {
	t.Parallel()

	db := openBenchDB(t)
	cs := NewCollectorStore(db)
	c := createCollector(t, cs)
	stuck := must(cs.StartRun(t.Context(), c.ID, RunModeFull))(t)

	past := time.Now().Add(-RunAbandonTimeout - time.Minute)
	db.Model(&CollectorRun{}).Where("id = ?", stuck.ID).Update("started_at", past)
	db.Model(&Collector{}).Where("id = ?", c.ID).Update("last_seen_at", past)

	if got := must(cs.Get(t.Context(), c.ID))(t); got.Status != CollectorStatusFailed {
		t.Fatalf("status with abandoned run = %q, want failed", got.Status)
	}

	// The next start closes the abandoned run.
	next := must(cs.StartRun(t.Context(), c.ID, RunModeIncremental))(t)
	runs := must(cs.ListRuns(t.Context(), c.ID, 10))(t)

	if len(runs) != 2 || runs[0].ID != next.ID || runs[1].ID != stuck.ID {
		t.Fatalf("ListRuns = %+v, want runs %s, %s", runs, next.ID, stuck.ID)
	}

	if runs[1].State != RunStateFailed || runs[1].Error != abandonedRunError {
		t.Fatalf("abandoned run = %+v, want it closed as failed", runs[1])
	}
}

// tagRows marks the rows of model with the given IDs as last written by the
// collector in the run.
func tagRows(t *testing.T, db *gorm.DB, model any, collectorID, runID uuid.UUID, ids ...uuid.UUID) {
	t.Helper()

	err := db.Model(model).Where("id IN ?", ids).
		Updates(map[string]any{"collector_id": collectorID, "last_run_id": runID}).Error
	if err != nil {
		t.Fatal(err)
	}
}

// rowExists reports whether model has a row with the given ID.
func rowExists(t *testing.T, db *gorm.DB, model any, id uuid.UUID) bool {
	t.Helper()

	var n int64
	if err := db.Model(model).Where("id = ?", id).Count(&n).Error; err != nil {
		t.Fatal(err)
	}

	return n > 0
}

// finishWithoutDeleting finishes the run and fails t if that deleted
// anything.
func finishWithoutDeleting(t *testing.T, cs *CollectorStore, c Collector, run CollectorRun, state RunState) {
	t.Helper()

	if finished := must(cs.FinishRun(t.Context(), c.ID, run.ID, state, ""))(t); finished.Deleted != 0 {
		t.Fatalf("FinishRun of %s run as %s deleted %d rows, want 0", run.Mode, state, finished.Deleted)
	}
}

func TestCollectorStore_FullRunDeletesUnseen(t *testing.T) {
	t.Parallel()

	db := openBenchDB(t)
	cs := NewCollectorStore(db)
	c := createCollector(t, cs)
	tag := uuid.NewString()

	results := createBulk(t, db, []BulkStreetInput{
		bulkStreet("Kept "+tag, 1, 2),   // sent again in the full run
		bulkStreet("Gone "+tag, 1),      // not sent again
		bulkStreet("Shared "+tag, 1, 2), // not sent again, but house number 2 is manual
		bulkStreet("Manual "+tag, 1),    // never a collector's
		bulkStreet("Seen only " + tag),  // street sent again, house number not
		bulkStreet("Geo " + tag),        // not sent again, holds a manual geo object
		bulkStreet("Geo owned "+tag, 1), // its geo object is the collector's and not sent
	})
	kept, gone, shared, manual, seenOnly, geo, geoOwned := results[0], results[1], results[2], results[3], results[4], results[5], results[6]

	geoObjects := NewGeoObjectStore(db)
	manualGeo := must(geoObjects.Create(t.Context(), GeoObject{Name: "manual " + tag, Group: "g", StreetID: geo.Street.ID, Latitude: 50, Longitude: 7}))(t)
	ownedGeo := must(geoObjects.Create(t.Context(), GeoObject{Name: "owned " + tag, Group: "g", StreetID: geoOwned.Street.ID, HouseNumberID: &geoOwned.HouseNumbers[0].ID, Latitude: 50, Longitude: 7}))(t)

	first := must(cs.StartRun(t.Context(), c.ID, RunModeIncremental))(t)

	for _, r := range []BulkResult{kept, gone, seenOnly, geo, geoOwned} {
		tagRows(t, db, &Street{}, c.ID, first.ID, r.Street.ID)

		for _, h := range r.HouseNumbers {
			tagRows(t, db, &HouseNumber{}, c.ID, first.ID, h.ID)
		}
	}

	tagRows(t, db, &Street{}, c.ID, first.ID, shared.Street.ID)
	tagRows(t, db, &HouseNumber{}, c.ID, first.ID, shared.HouseNumbers[0].ID)
	tagRows(t, db, &GeoObject{}, c.ID, first.ID, ownedGeo.ID)

	finishWithoutDeleting(t, cs, c, first, RunStateSucceeded)

	sendKept := func(run CollectorRun) {
		tagRows(t, db, &Street{}, c.ID, run.ID, kept.Street.ID, seenOnly.Street.ID)
		tagRows(t, db, &HouseNumber{}, c.ID, run.ID, kept.HouseNumbers[0].ID, kept.HouseNumbers[1].ID)
	}

	failed := must(cs.StartRun(t.Context(), c.ID, RunModeFull))(t)
	sendKept(failed)
	finishWithoutDeleting(t, cs, c, failed, RunStateFailed)

	full := must(cs.StartRun(t.Context(), c.ID, RunModeFull))(t)
	sendKept(full)
	run := must(cs.FinishRun(t.Context(), c.ID, full.ID, RunStateSucceeded, ""))(t)

	checks := []struct {
		what   string
		model  any
		id     uuid.UUID
		exists bool
	}{
		{"street sent in the run", &Street{}, kept.Street.ID, true},
		{"house number sent in the run", &HouseNumber{}, kept.HouseNumbers[0].ID, true},
		{"street not sent", &Street{}, gone.Street.ID, false},
		{"house number not sent", &HouseNumber{}, gone.HouseNumbers[0].ID, false},
		{"collector house number on a street with manual data", &HouseNumber{}, shared.HouseNumbers[0].ID, false},
		{"manual house number", &HouseNumber{}, shared.HouseNumbers[1].ID, true},
		{"street still holding manual data", &Street{}, shared.Street.ID, true},
		{"manual street", &Street{}, manual.Street.ID, true},
		{"street sent without its house numbers", &Street{}, seenOnly.Street.ID, true},
		{"street holding a manual geo object", &Street{}, geo.Street.ID, true},
		{"manual geo object", &GeoObject{}, manualGeo.ID, true},
		{"collector geo object not sent", &GeoObject{}, ownedGeo.ID, false},
		{"street whose data was all unseen", &Street{}, geoOwned.Street.ID, false},
	}

	for _, check := range checks {
		if got := rowExists(t, db, check.model, check.id); got != check.exists {
			t.Errorf("%s: exists = %v, want %v", check.what, got, check.exists)
		}
	}

	// gone: street and house number; shared: one house number; geoOwned:
	// geo object, house number and street.
	if run.Deleted != 6 {
		t.Errorf("run.Deleted = %d, want 6", run.Deleted)
	}
}

func TestCollectorStore_DeleteKeepsData(t *testing.T) {
	t.Parallel()

	db := openBenchDB(t)
	cs := NewCollectorStore(db)
	c := createCollector(t, cs)
	street := createBulk(t, db, []BulkStreetInput{bulkStreet("Orphan " + uuid.NewString())})[0].Street
	run := must(cs.StartRun(t.Context(), c.ID, RunModeIncremental))(t)
	tagRows(t, db, &Street{}, c.ID, run.ID, street.ID)

	wantErrorIs(t, "Delete", cs.Delete(t.Context(), c.ID), nil)

	var s Street
	if err := db.Take(&s, "id = ?", street.ID).Error; err != nil {
		t.Fatal(err)
	}

	if s.CollectorID != nil {
		t.Fatalf("street of deleted collector has collector_id %v, want NULL", *s.CollectorID)
	}
}

func TestCollectorStore_PrunesRuns(t *testing.T) {
	t.Parallel()

	cs := NewCollectorStore(openBenchDB(t))
	c := createCollector(t, cs)

	for range maxRunsPerCollector + 3 {
		run := must(cs.StartRun(t.Context(), c.ID, RunModeIncremental))(t)
		must(cs.FinishRun(t.Context(), c.ID, run.ID, RunStateSucceeded, ""))(t)
	}

	if runs := must(cs.ListRuns(t.Context(), c.ID, 1000))(t); len(runs) != maxRunsPerCollector {
		t.Fatalf("kept %d runs, want %d", len(runs), maxRunsPerCollector)
	}
}

func TestCollectorStore_Token(t *testing.T) {
	t.Parallel()

	db := openBenchDB(t)
	cs := NewCollectorStore(db)
	tokens := NewAPITokenStore(db)
	owner := "owner-" + uuid.NewString()

	c, secret, err := cs.Create(t.Context(), owner, "Owner", CollectorInput{Name: "collector " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = cs.Delete(context.Background(), c.ID) })

	token := must(tokens.Authenticate(t.Context(), secret))(t)
	if token.CollectorID == nil || *token.CollectorID != c.ID {
		t.Fatalf("collector token has collector %v, want %s", token.CollectorID, c.ID)
	}

	if seen := must(cs.Get(t.Context(), c.ID))(t); seen.LastSeenAt == nil {
		t.Fatal("authenticating did not record contact from the collector")
	}

	if list := must(tokens.List(t.Context(), owner))(t); len(list) != 0 {
		t.Fatalf("owner's token list = %+v, want collector token left out", list)
	}

	wantErrorIs(t, "Delete as a personal token", tokens.Delete(t.Context(), owner, token.ID), ErrNotFound)

	rotated := must(cs.RotateToken(t.Context(), c.ID, owner, "Owner"))(t)

	_, err = tokens.Authenticate(t.Context(), secret)
	wantErrorIs(t, "old secret after rotation", err, ErrInvalidAPIToken)

	must(tokens.Authenticate(t.Context(), rotated))(t)

	_, err = cs.RotateToken(t.Context(), uuid.New(), owner, "Owner")
	wantErrorIs(t, "RotateToken of unknown collector", err, ErrNotFound)

	wantErrorIs(t, "Delete", cs.Delete(t.Context(), c.ID), nil)

	_, err = tokens.Authenticate(t.Context(), rotated)
	wantErrorIs(t, "secret of deleted collector", err, ErrInvalidAPIToken)
}
