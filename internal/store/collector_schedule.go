package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TriggeredBySchedule is the CollectorRun.TriggeredBy of runs the schedule
// started.
const TriggeredBySchedule = "schedule"

// minScheduleInterval is the shortest "@every" interval a schedule may use.
// The scheduler checks for due runs a few times a minute, so anything
// shorter would not run as often as it says.
const minScheduleInterval = time.Minute

// ErrInvalidSchedule is wrapped by the errors of schedules that can't be
// parsed.
var ErrInvalidSchedule = errors.New("invalid schedule")

// scheduleParser accepts the five standard cron fields (minute, hour,
// day-of-month, month, day-of-week), descriptors such as "@daily" and "@every 6h",
// and a "CRON_TZ=Europe/Berlin " prefix.
var scheduleParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// ParseSchedule parses a cron expression; see Collector.Schedule. Errors
// wrap ErrInvalidSchedule.
func ParseSchedule(expr string) (cron.Schedule, error) {
	s, err := scheduleParser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSchedule, err)
	}

	if every, ok := s.(cron.ConstantDelaySchedule); ok && every.Delay < minScheduleInterval {
		return nil, fmt.Errorf("%w: @every must be at least %s", ErrInvalidSchedule, minScheduleInterval)
	}

	return s, nil
}

// NextScheduledRun returns when the schedule next starts a run, which is in
// the past while a due run waits for the scheduler to pick it up. It is
// false for a collector without a valid schedule.
func (c Collector) NextScheduledRun() (time.Time, bool) {
	if c.Schedule == "" {
		return time.Time{}, false
	}

	s, err := ParseSchedule(c.Schedule)
	if err != nil {
		return time.Time{}, false
	}

	return s.Next(c.scheduleBase()), true
}

// scheduleBase is the time after which the schedule's next run is due.
func (c Collector) scheduleBase() time.Time {
	if c.ScheduledUntil != nil {
		return *c.ScheduledUntil
	}

	return c.UpdatedAt
}

// UpdateSchedule sets the collector's schedule, or removes it for an empty
// expr, and returns the updated collector. Runs are due from now on, so a
// new schedule never starts runs for times that have already passed. It
// returns ErrNotFound for an unknown collector and an error wrapping
// ErrInvalidSchedule for an expression that doesn't parse.
func (cs *CollectorStore) UpdateSchedule(ctx context.Context, id, expr string) (Collector, error) {
	expr = strings.TrimSpace(expr)
	if expr != "" {
		if _, err := ParseSchedule(expr); err != nil {
			return Collector{}, err
		}
	}

	var c Collector

	now := time.Now()

	err := cs.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Collector{}).Where("id = ?", id).Updates(map[string]any{
			"schedule": expr, "scheduled_until": now, "updated_at": now,
		})
		if err := requireOneRow(res); err != nil {
			return err
		}

		return tx.Take(&c, "id = ?", id).Error
	})
	if err != nil {
		return Collector{}, err
	}

	return c, nil
}

// DueScheduledRuns returns the collectors whose schedule has a run due at
// now. Each is still to be claimed with ClaimScheduledRun.
func (cs *CollectorStore) DueScheduledRuns(ctx context.Context, now time.Time) ([]Collector, error) {
	var scheduled []Collector

	err := cs.db.WithContext(ctx).Where("schedule <> ''").Order("id").Find(&scheduled).Error
	if err != nil {
		return nil, err
	}

	due := scheduled[:0]

	for _, c := range scheduled {
		if next, ok := c.NextScheduledRun(); ok && !next.After(now) {
			due = append(due, c)
		}
	}

	return due, nil
}

// ClaimScheduledRun marks the due run of c, as DueScheduledRuns returned it,
// as handled up to now. It is false when another instance claimed it first,
// or the schedule changed since c was read, so that each due run starts at
// most once. Runs missed while no instance ran collapse into this one.
func (cs *CollectorStore) ClaimScheduledRun(ctx context.Context, c Collector, now time.Time) (bool, error) {
	res := cs.db.WithContext(ctx).Model(&Collector{}).
		Where("id = ? AND schedule = ?", c.ID, c.Schedule).
		Where(clause.Expr{SQL: "scheduled_until IS NOT DISTINCT FROM ?", Vars: []any{c.ScheduledUntil}}).
		UpdateColumn("scheduled_until", now)
	if res.Error != nil {
		return false, fmt.Errorf("claim scheduled run of %s: %w", c.ID, res.Error)
	}

	return res.RowsAffected == 1, nil
}
