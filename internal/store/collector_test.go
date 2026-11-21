package store

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

func describedAs(name string, settings ...contract.SettingSpec) *contract.Description {
	return &contract.Description{
		ContractVersion: contract.Version, Name: name, Version: "1",
		Description: contract.LocalizedText{"en": name}, Settings: settings,
	}
}

// checkDiscovered checks the collectors TestCollectorStore_SaveDiscovered
// leaves after its second scan.
func checkDiscovered(t *testing.T, cs *CollectorStore) {
	t.Helper()

	collectors, err := cs.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]Collector{}
	for _, c := range collectors {
		byID[c.ID] = c
	}

	a := byID["a"]
	if a.Status != CollectorInvalid || deref(a.Error) != "describe failed" || a.Path != "/c/a.py" {
		t.Errorf("a = %+v, want invalid at its new path", a)
	}

	if a.Name != "A" || len(a.SettingsSchema) != 1 || string(a.Settings["endpoint"]) != `"https://x"` {
		t.Errorf("a = %+v, want its last valid metadata and its settings kept", a)
	}

	if byID["b"].Status != CollectorMissing || byID["c"].Status != CollectorOK || byID["c"].Name != "C" {
		t.Errorf("b = %s, c = %s %q; want missing and ok", byID["b"].Status, byID["c"].Status, byID["c"].Name)
	}
}

// TestCollectorStore_SaveDiscovered runs in an isolated schema: marking
// collectors missing touches every collector, including other tests'.
func TestCollectorStore_SaveDiscovered(t *testing.T) {
	t.Parallel()

	db := openIsolatedDB(t)
	if err := Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	cs := NewCollectorStore(db)
	ctx := t.Context()
	endpoint := contract.SettingSpec{Key: "endpoint", Label: contract.LocalizedText{"": "Endpoint"}, Type: contract.SettingString}

	err := cs.SaveDiscovered(ctx, []Discovered{
		{ID: "a", Path: "/c/a", Description: describedAs("A", endpoint)},
		{ID: "b", Path: "/c/b", Description: describedAs("B")},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := cs.UpdateSettings(ctx, "a", SettingsInput{"endpoint": json.RawMessage(`"https://x"`)}); err != nil {
		t.Fatal(err)
	}

	// a breaks, b disappears, c is new.
	err = cs.SaveDiscovered(ctx, []Discovered{
		{ID: "a", Path: "/c/a.py", Error: "describe failed"},
		{ID: "c", Path: "/c/c", Description: describedAs("C")},
	})
	if err != nil {
		t.Fatal(err)
	}

	checkDiscovered(t, cs)

	// A collector that comes back is ok again, with its error cleared.
	if err := cs.SaveDiscovered(ctx, []Discovered{{ID: "a", Path: "/c/a", Description: describedAs("A2")}}); err != nil {
		t.Fatal(err)
	}

	if a, _ := cs.Get(ctx, "a"); a.Status != CollectorOK || a.Error != nil || a.Name != "A2" {
		t.Errorf("a = %+v, want ok again", a)
	}
}

func TestCollectorStore_UpdateSettings(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	ctx := t.Context()

	err := f.db.Model(&Collector{}).Where("id = ?", f.id).Update("settings_schema", jsonValue{[]contract.SettingSpec{
		{Key: "token", Type: contract.SettingSecret},
	}}).Error
	if err != nil {
		t.Fatal(err)
	}

	c, err := f.cs.UpdateSettings(ctx, f.id, SettingsInput{"token": json.RawMessage(`"s"`)})
	if err != nil || string(c.Settings["token"]) != `"s"` {
		t.Fatalf("UpdateSettings = %+v, %v", c.Settings, err)
	}

	if _, err := f.cs.UpdateSettings(ctx, f.id, SettingsInput{"other": json.RawMessage(`1`)}); !errors.Is(err, ErrInvalidSettings) {
		t.Errorf("unknown setting: err = %v, want ErrInvalidSettings", err)
	}

	if _, err := f.cs.UpdateSettings(ctx, "nope-"+f.id, SettingsInput{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown collector: err = %v, want ErrNotFound", err)
	}
}

func TestCollectorStore_Delete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     CollectorStatus
		running    bool
		deleteData bool
		wantErr    error
		wantStreet string // "owned" (unchanged), "manual" or "gone"
	}{
		{name: "available collector", status: CollectorOK, wantErr: ErrCollectorAvailable, wantStreet: "owned"},
		{name: "while running", status: CollectorMissing, running: true, wantErr: ErrRunActive, wantStreet: "owned"},
		{name: "keeps rows as manual", status: CollectorMissing, wantStreet: "manual"},
		{name: "deletes rows", status: CollectorInvalid, deleteData: true, wantStreet: "gone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newSyncFixture(t)
			f.sync(t, contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0)}})

			if err := f.db.Model(&Collector{}).Where("id = ?", f.id).Update("status", tt.status).Error; err != nil {
				t.Fatal(err)
			}

			if tt.running {
				if _, err := f.cs.CreateRun(t.Context(), f.id, "test"); err != nil {
					t.Fatal(err)
				}
			}

			if err := f.cs.Delete(t.Context(), f.id, tt.deleteData); !errors.Is(err, tt.wantErr) {
				t.Fatalf("Delete = %v, want %v", err, tt.wantErr)
			}

			var streets []Street
			if err := f.db.Where("city = ?", f.city).Find(&streets).Error; err != nil {
				t.Fatal(err)
			}

			got := "gone"
			if len(streets) == 1 {
				got = "manual"
				if streets[0].CollectorID != nil {
					got = "owned"
				}
			}

			if got != tt.wantStreet {
				t.Errorf("street is %s, want %s", got, tt.wantStreet)
			}
		})
	}
}

// runOnce starts, advances and finishes a run of the collector, reporting
// created streets, and returns its ID. It keeps the newest 3 runs.
func runOnce(t *testing.T, cs *CollectorStore, collectorID string, created int) uuid.UUID {
	t.Helper()

	ctx := t.Context()

	run, err := cs.CreateRun(ctx, collectorID, "tester")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := cs.CreateRun(ctx, collectorID, "tester"); !errors.Is(err, ErrRunActive) {
		t.Fatalf("second CreateRun = %v, want ErrRunActive", err)
	}

	if err := cs.SetRunPhase(ctx, run.ID, PhaseApplying); err != nil {
		t.Fatal(err)
	}

	finished, err := cs.FinishRun(ctx, run.ID, RunOutcome{
		Status: RunSucceeded, ExitCode: new(0), Stats: &SyncStats{Streets: EntityStats{Created: created}},
		Issues: []SyncIssue{{Kind: IssueSkipped, Entity: EntityAddress, Key: "k"}}, Log: "log",
	}, 3)
	if err != nil {
		t.Fatal(err)
	}

	if finished.Status != RunSucceeded || finished.Phase != nil || finished.FinishedAt == nil || finished.Stats.Streets.Created != created {
		t.Errorf("finished run = %+v", finished)
	}

	return run.ID
}

func TestCollectorStore_Runs(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	ctx := t.Context()

	var ids []uuid.UUID

	for i := range 4 {
		ids = append(ids, runOnce(t, f.cs, f.id, i))
	}

	runs, err := f.cs.ListRuns(ctx, f.id, 10, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(runs) != 3 || runs[0].ID != ids[3] || runs[0].Log != "" || runs[0].Issues != nil {
		t.Errorf("runs = %+v, want the newest 3 without issues and logs", runs)
	}

	if _, err := f.cs.GetRun(ctx, ids[0]); !errors.Is(err, ErrNotFound) {
		t.Errorf("oldest run: err = %v, want it pruned", err)
	}

	run, err := f.cs.GetRun(ctx, ids[3])
	if err != nil || run.Log != "log" || len(run.Issues) != 1 {
		t.Errorf("GetRun = %+v, %v", run, err)
	}
}

func TestCollectorStore_LastRuns(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	runOnce(t, f.cs, f.id, 1)
	newest := runOnce(t, f.cs, f.id, 2)

	last, err := f.cs.LastRuns(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if got := last[f.id]; got.ID != newest || got.Stats == nil || got.Stats.Streets.Created != 2 {
		t.Errorf("last run = %+v, want %v with its stats", got, newest)
	}

	if _, err := f.cs.CreateRun(t.Context(), "unknown-"+f.id, "tester"); !errors.Is(err, ErrNotFound) {
		t.Errorf("CreateRun for an unknown collector = %v, want ErrNotFound", err)
	}
}

// TestCollectorStore_FailInterruptedRuns runs in an isolated schema: it fails
// every running run, including other tests'.
func TestCollectorStore_FailInterruptedRuns(t *testing.T) {
	t.Parallel()

	db := openIsolatedDB(t)
	if err := Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	cs := NewCollectorStore(db)
	if err := cs.SaveDiscovered(t.Context(), []Discovered{{ID: "a", Path: "/a", Description: describedAs("A")}}); err != nil {
		t.Fatal(err)
	}

	run, err := cs.CreateRun(t.Context(), "a", "tester")
	if err != nil {
		t.Fatal(err)
	}

	if n, err := cs.FailInterruptedRuns(t.Context()); err != nil || n != 1 {
		t.Fatalf("FailInterruptedRuns = %d, %v; want 1", n, err)
	}

	if run, _ = cs.GetRun(t.Context(), run.ID); run.Status != RunFailed || deref(run.Error) != interruptedRunError {
		t.Errorf("run = %+v, want failed as interrupted", run)
	}

	// The collector can run again.
	if _, err := cs.CreateRun(t.Context(), "a", "tester"); err != nil {
		t.Errorf("CreateRun after recovery: %v", err)
	}
}
