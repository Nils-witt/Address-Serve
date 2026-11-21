package collector

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/nils-witt/address-serve/internal/store"
)

// scheduleCheckInterval is how often RunSchedules looks for due runs, unless
// Config.ScheduleCheckInterval says otherwise. Cron schedules have minute
// resolution, so runs start at most this late.
const scheduleCheckInterval = 15 * time.Second

// RunSchedules starts the runs that collectors' schedules make due, checking
// every Config.ScheduleCheckInterval until ctx is done. Several instances
// may run it at once: each due run is claimed in the database and starts on
// one of them only.
func (r *Runner) RunSchedules(ctx context.Context) {
	interval := r.cfg.ScheduleCheckInterval
	if interval <= 0 {
		interval = scheduleCheckInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		r.StartDueRuns(ctx, time.Now())

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// StartDueRuns starts a run of each collector whose schedule has one due at
// now. A due run that can't start, because the collector is unavailable,
// lacks settings or is still running, is logged and skipped, not retried:
// the schedule's next time comes soon enough.
func (r *Runner) StartDueRuns(ctx context.Context, now time.Time) {
	due, err := r.store.DueScheduledRuns(ctx, now)
	if err != nil {
		log.Printf("collector schedules: %v", err)
		return
	}

	for _, c := range due {
		claimed, err := r.store.ClaimScheduledRun(ctx, c, now)
		if err != nil {
			log.Printf("collector %s: %v", c.ID, err)
			continue
		}

		if !claimed {
			continue
		}

		run, err := r.Start(ctx, c.ID, store.TriggeredBySchedule)

		var missing *MissingSettingsError

		switch {
		case errors.Is(err, store.ErrRunActive):
			log.Printf("collector %s: scheduled run skipped, the previous run is still going", c.ID)
		case errors.As(err, &missing), errors.Is(err, ErrUnavailable):
			log.Printf("collector %s: scheduled run skipped: %v", c.ID, err)
		case err != nil:
			log.Printf("collector %s: start scheduled run: %v", c.ID, err)
		default:
			log.Printf("collector %s: scheduled run %s started", c.ID, run.ID)
		}
	}
}
