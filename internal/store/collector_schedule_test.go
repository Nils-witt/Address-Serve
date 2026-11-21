package store

import (
	"errors"
	"testing"
	"time"
)

func TestParseSchedule(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 11, 21, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		expr     string
		wantNext time.Time // zero: invalid
	}{
		{expr: "0 3 * * *", wantNext: time.Date(2026, 11, 22, 3, 0, 0, 0, time.UTC)},
		{expr: "*/15 * * * *", wantNext: time.Date(2026, 11, 21, 10, 45, 0, 0, time.UTC)},
		{expr: "@hourly", wantNext: time.Date(2026, 11, 21, 11, 0, 0, 0, time.UTC)},
		{expr: "@every 6h", wantNext: base.Add(6 * time.Hour)},
		{expr: "CRON_TZ=Europe/Berlin 0 3 * * *", wantNext: time.Date(2026, 11, 22, 2, 0, 0, 0, time.UTC)},
		{expr: "@every 30s"},
		{expr: "0 0 3 * * *"}, // seconds field
		{expr: "61 * * * *"},
		{expr: "CRON_TZ=Nowhere/Nope 0 3 * * *"},
		{expr: "daily"},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			t.Parallel()

			s, err := ParseSchedule(tt.expr)
			if tt.wantNext.IsZero() {
				if !errors.Is(err, ErrInvalidSchedule) {
					t.Fatalf("err = %v, want ErrInvalidSchedule", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if got := s.Next(base); !got.Equal(tt.wantNext) {
				t.Errorf("Next = %s, want %s", got, tt.wantNext)
			}
		})
	}
}

func TestCollector_NextScheduledRun(t *testing.T) {
	t.Parallel()

	updated := time.Date(2026, 11, 21, 10, 30, 0, 0, time.UTC)
	claimed := time.Date(2026, 11, 22, 3, 0, 5, 0, time.UTC)

	tests := []struct {
		name   string
		c      Collector
		want   time.Time
		wantOK bool
	}{
		{name: "no schedule", c: Collector{UpdatedAt: updated}},
		{name: "invalid schedule", c: Collector{Schedule: "nope", UpdatedAt: updated}},
		{
			name: "after it was set", c: Collector{Schedule: "0 3 * * *", UpdatedAt: updated},
			want: time.Date(2026, 11, 22, 3, 0, 0, 0, time.UTC), wantOK: true,
		},
		{
			name: "after its last claim", c: Collector{Schedule: "0 3 * * *", UpdatedAt: updated, ScheduledUntil: &claimed},
			want: time.Date(2026, 11, 23, 3, 0, 0, 0, time.UTC), wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := tt.c.NextScheduledRun()
			if ok != tt.wantOK || !got.Equal(tt.want) {
				t.Errorf("NextScheduledRun = %s, %v; want %s, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// isDue returns whether DueScheduledRuns reports the collector due at now,
// and the collector as it returned it.
func isDue(t *testing.T, cs *CollectorStore, id string, now time.Time) (Collector, bool) {
	t.Helper()

	due, err := cs.DueScheduledRuns(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range due {
		if c.ID == id {
			return c, true
		}
	}

	return Collector{}, false
}

func TestCollectorStore_UpdateSchedule(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	ctx := t.Context()

	if _, err := f.cs.UpdateSchedule(ctx, f.id, "nope"); !errors.Is(err, ErrInvalidSchedule) {
		t.Errorf("invalid schedule: err = %v, want ErrInvalidSchedule", err)
	}

	if _, err := f.cs.UpdateSchedule(ctx, "nope-"+f.id, "@hourly"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown collector: err = %v, want ErrNotFound", err)
	}

	c, err := f.cs.UpdateSchedule(ctx, f.id, "  @every 1h ")
	if err != nil || c.Schedule != "@every 1h" || c.ScheduledUntil == nil {
		t.Fatalf("UpdateSchedule = %+v, %v", c, err)
	}

	set := *c.ScheduledUntil

	if _, due := isDue(t, f.cs, f.id, set.Add(59*time.Minute)); due {
		t.Error("due before its time")
	}

	if _, due := isDue(t, f.cs, f.id, set.Add(time.Hour)); !due {
		t.Error("not due at its time")
	}

	c, err = f.cs.UpdateSchedule(ctx, f.id, "")
	if err != nil || c.Schedule != "" {
		t.Fatalf("remove schedule = %+v, %v", c, err)
	}

	if _, due := isDue(t, f.cs, f.id, set.Add(48*time.Hour)); due {
		t.Error("due without a schedule")
	}
}

func TestCollectorStore_ClaimScheduledRun(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	ctx := t.Context()

	c, err := f.cs.UpdateSchedule(ctx, f.id, "@every 1h")
	if err != nil {
		t.Fatal(err)
	}

	// Missed runs collapse into one claim, which only one caller wins.
	later := c.ScheduledUntil.Add(5 * time.Hour)
	c, _ = isDue(t, f.cs, f.id, later)

	for i, want := range []bool{true, false} {
		claimed, err := f.cs.ClaimScheduledRun(ctx, c, later)
		if err != nil || claimed != want {
			t.Errorf("claim %d = %v, %v; want %v", i, claimed, err, want)
		}
	}

	if _, due := isDue(t, f.cs, f.id, later.Add(59*time.Minute)); due {
		t.Error("due again before the next time")
	}

	// A claim of a schedule that changed since it was read fails.
	stale, _ := isDue(t, f.cs, f.id, later.Add(time.Hour))

	if _, err := f.cs.UpdateSchedule(ctx, f.id, "@daily"); err != nil {
		t.Fatal(err)
	}

	if claimed, err := f.cs.ClaimScheduledRun(ctx, stale, later.Add(time.Hour)); err != nil || claimed {
		t.Errorf("stale claim = %v, %v; want false", claimed, err)
	}
}
