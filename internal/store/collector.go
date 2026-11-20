package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RunMode tells how a collector run treats data it does not send.
type RunMode string

const (
	// RunModeIncremental only adds and updates data.
	RunModeIncremental RunMode = "incremental"
	// RunModeFull also deletes the collector's data not sent in the run, once
	// the run succeeds.
	RunModeFull RunMode = "full"
)

// Valid reports whether m is a known run mode.
func (m RunMode) Valid() bool {
	return m == RunModeIncremental || m == RunModeFull
}

// RunState is the lifecycle state of a collector run.
type RunState string

// The states of a run: it is running until its collector finishes it.
const (
	RunStateRunning   RunState = "running"
	RunStateSucceeded RunState = "succeeded"
	RunStateFailed    RunState = "failed"
)

// ValidFinal reports whether s is a state a run can be finished with.
func (s RunState) ValidFinal() bool {
	return s == RunStateSucceeded || s == RunStateFailed
}

// CollectorStatus summarises a collector's health for display. It is
// derived from its last run, last contact and expected interval; see
// collectorStatus.
type CollectorStatus string

// The collector statuses, as decided by collectorStatus.
const (
	CollectorStatusNever   CollectorStatus = "never"
	CollectorStatusRunning CollectorStatus = "running"
	CollectorStatusOK      CollectorStatus = "ok"
	CollectorStatusFailed  CollectorStatus = "failed"
	CollectorStatusStale   CollectorStatus = "stale"
)

const (
	// RunAbandonTimeout is how long an open run may go without contact from
	// its collector before it counts as failed.
	RunAbandonTimeout = 30 * time.Minute

	// abandonedRunError is recorded on a run closed for lack of contact.
	abandonedRunError = "abandoned: no contact from the collector for 30 minutes"

	// maxRunsPerCollector bounds the run history kept per collector.
	maxRunsPerCollector = 50

	maxCollectorNameLen        = 100
	maxCollectorDescriptionLen = 500
	maxExpectedIntervalMinutes = 366 * 24 * 60
	maxRunErrorLen             = 2000
)

// Collector is an external system that feeds streets, house numbers and geo
// objects into the store, in runs.
type Collector struct {
	ID          uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name        string    `json:"name" gorm:"not null;uniqueIndex:collectors_name_key"`
	Description string    `json:"description" gorm:"not null;default:''"`
	// ExpectedIntervalMinutes, when set, is how often the collector should
	// complete a run; it is stale once its last success is twice as old.
	ExpectedIntervalMinutes *int       `json:"expectedIntervalMinutes"`
	OwnerSub                string     `json:"-" gorm:"not null"`
	CreatedAt               time.Time  `json:"createdAt" gorm:"not null"`
	LastSeenAt              *time.Time `json:"lastSeenAt"`
	LastSuccessAt           *time.Time `json:"lastSuccessAt"`

	// Status and LastRun are filled in on read.
	Status  CollectorStatus `json:"status" gorm:"-"`
	LastRun *CollectorRun   `json:"lastRun" gorm:"-"`

	// The slices below are never loaded; they declare the foreign keys to
	// collectors. Deleting a collector deletes its runs and keeps the data it
	// wrote, which no longer belongs to any collector.
	Runs         []CollectorRun `json:"-" gorm:"constraint:OnDelete:CASCADE"`
	Streets      []Street       `json:"-" gorm:"constraint:OnDelete:SET NULL"`
	HouseNumbers []HouseNumber  `json:"-" gorm:"constraint:OnDelete:SET NULL"`
	GeoObjects   []GeoObject    `json:"-" gorm:"constraint:OnDelete:SET NULL"`
}

// CollectorRun is one pass of a collector over its data. The row counts are
// kept by the server as the collector writes.
type CollectorRun struct {
	ID           uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CollectorID  uuid.UUID  `json:"collectorId" gorm:"type:uuid;not null;index:collector_runs_collector_started_idx,priority:1"`
	Mode         RunMode    `json:"mode" gorm:"type:text;not null"`
	State        RunState   `json:"state" gorm:"type:text;not null"`
	StartedAt    time.Time  `json:"startedAt" gorm:"not null;index:collector_runs_collector_started_idx,priority:2"`
	FinishedAt   *time.Time `json:"finishedAt"`
	Streets      int        `json:"streets" gorm:"not null;default:0"`
	HouseNumbers int        `json:"houseNumbers" gorm:"not null;default:0"`
	GeoObjects   int        `json:"geoObjects" gorm:"not null;default:0"`
	Deleted      int        `json:"deleted" gorm:"not null;default:0"`
	Error        string     `json:"error,omitempty" gorm:"not null;default:''"`
}

// lastContact is the latest moment the run's collector is known to have
// been active.
func (r CollectorRun) lastContact(lastSeenAt *time.Time) time.Time {
	if lastSeenAt != nil && lastSeenAt.After(r.StartedAt) {
		return *lastSeenAt
	}

	return r.StartedAt
}

// collectorStatus derives c's status from c.LastRun, c.LastSeenAt and
// c.LastSuccessAt as of now.
func collectorStatus(c Collector, now time.Time) CollectorStatus {
	run := c.LastRun

	switch {
	case run == nil:
		return CollectorStatusNever
	case run.State == RunStateRunning:
		if now.Sub(run.lastContact(c.LastSeenAt)) > RunAbandonTimeout {
			return CollectorStatusFailed
		}

		return CollectorStatusRunning
	case run.State == RunStateFailed:
		return CollectorStatusFailed
	case c.ExpectedIntervalMinutes != nil:
		staleAfter := 2 * time.Duration(*c.ExpectedIntervalMinutes) * time.Minute
		if c.LastSuccessAt == nil || now.Sub(*c.LastSuccessAt) > staleAfter {
			return CollectorStatusStale
		}
	}

	return CollectorStatusOK
}

// CollectorInput holds the fields of a collector that users set.
type CollectorInput struct {
	Name                    string `json:"name"`
	Description             string `json:"description"`
	ExpectedIntervalMinutes *int   `json:"expectedIntervalMinutes"`
}

// normalized returns in with surrounding whitespace trimmed.
func (in CollectorInput) normalized() CollectorInput {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)

	return in
}

// Validate reports whether in has a name and fields within their limits.
func (in CollectorInput) Validate() error {
	in = in.normalized()

	if in.Name == "" {
		return errors.New("name is required")
	}

	if utf8.RuneCountInString(in.Name) > maxCollectorNameLen {
		return fmt.Errorf("name must be at most %d characters", maxCollectorNameLen)
	}

	if utf8.RuneCountInString(in.Description) > maxCollectorDescriptionLen {
		return fmt.Errorf("description must be at most %d characters", maxCollectorDescriptionLen)
	}

	if m := in.ExpectedIntervalMinutes; m != nil && (*m < 1 || *m > maxExpectedIntervalMinutes) {
		return fmt.Errorf("expectedIntervalMinutes must be omitted or between 1 and %d", maxExpectedIntervalMinutes)
	}

	return nil
}

var (
	// ErrCollectorNameTaken is returned when another collector already has
	// the name.
	ErrCollectorNameTaken = errors.New("a collector with this name already exists")

	// ErrRunInProgress is returned when a collector starts a run while
	// another of its runs is still open.
	ErrRunInProgress = errors.New("the collector already has a running run")

	// ErrRunNotRunning is returned when a run that has already finished is
	// finished again or written to.
	ErrRunNotRunning = errors.New("the run is not running")
)

// CollectorStore reads and writes collectors and their runs.
type CollectorStore struct {
	db *gorm.DB
}

// NewCollectorStore returns a CollectorStore backed by db.
func NewCollectorStore(db *gorm.DB) *CollectorStore {
	return &CollectorStore{db: db}
}

// Create inserts a collector owned by ownerSub. It returns
// ErrCollectorNameTaken if the name is in use.
func (st *CollectorStore) Create(ctx context.Context, ownerSub string, in CollectorInput) (Collector, error) {
	in = in.normalized()
	c := Collector{
		Name:                    in.Name,
		Description:             in.Description,
		ExpectedIntervalMinutes: in.ExpectedIntervalMinutes,
		OwnerSub:                ownerSub,
		CreatedAt:               time.Now().UTC(),
	}

	err := st.db.WithContext(ctx).Create(&c).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return Collector{}, ErrCollectorNameTaken
	}

	if err != nil {
		return Collector{}, err
	}

	c.Status = collectorStatus(c, time.Now())

	return c, nil
}

// List returns every collector with its status and last run, ordered by
// name.
func (st *CollectorStore) List(ctx context.Context) ([]Collector, error) {
	db := st.db.WithContext(ctx)

	collectors := []Collector{}
	if err := db.Order("name").Find(&collectors).Error; err != nil {
		return nil, err
	}

	var lastRuns []CollectorRun

	err := db.Raw(`SELECT DISTINCT ON (collector_id) * FROM collector_runs
		ORDER BY collector_id, started_at DESC`).Scan(&lastRuns).Error
	if err != nil {
		return nil, fmt.Errorf("load last runs: %w", err)
	}

	byCollector := make(map[uuid.UUID]*CollectorRun, len(lastRuns))
	for i := range lastRuns {
		byCollector[lastRuns[i].CollectorID] = &lastRuns[i]
	}

	now := time.Now()

	for i := range collectors {
		collectors[i].LastRun = byCollector[collectors[i].ID]
		collectors[i].Status = collectorStatus(collectors[i], now)
	}

	return collectors, nil
}

// Get returns the collector with the given ID, with its status and last run,
// or ErrNotFound.
func (st *CollectorStore) Get(ctx context.Context, id uuid.UUID) (Collector, error) {
	db := st.db.WithContext(ctx)

	var c Collector
	if err := db.Take(&c, "id = ?", id).Error; err != nil {
		return Collector{}, notFound(err)
	}

	var run CollectorRun

	err := db.Where("collector_id = ?", id).Order("started_at DESC").Take(&run).Error

	switch {
	case err == nil:
		c.LastRun = &run
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return Collector{}, fmt.Errorf("load last run: %w", err)
	}

	c.Status = collectorStatus(c, time.Now())

	return c, nil
}

// Update replaces the user-set fields of the collector with the given ID. It
// returns ErrNotFound if it does not exist and ErrCollectorNameTaken if the
// name is in use.
func (st *CollectorStore) Update(ctx context.Context, id uuid.UUID, in CollectorInput) (Collector, error) {
	in = in.normalized()

	res := st.db.WithContext(ctx).Model(&Collector{}).Where("id = ?", id).Updates(map[string]any{
		"name":                      in.Name,
		"description":               in.Description,
		"expected_interval_minutes": in.ExpectedIntervalMinutes,
	})
	if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
		return Collector{}, ErrCollectorNameTaken
	}

	if err := requireOneRow(res); err != nil {
		return Collector{}, err
	}

	return st.Get(ctx, id)
}

// Delete removes the collector with the given ID and its runs. The data it
// wrote stays. It returns ErrNotFound if there is no such collector.
func (st *CollectorStore) Delete(ctx context.Context, id uuid.UUID) error {
	return requireOneRow(st.db.WithContext(ctx).Delete(&Collector{}, "id = ?", id))
}

// Touch records contact from the collector with the given ID, or returns
// ErrNotFound.
func (st *CollectorStore) Touch(ctx context.Context, id uuid.UUID) error {
	res := st.db.WithContext(ctx).Model(&Collector{}).Where("id = ?", id).Update("last_seen_at", time.Now().UTC())

	return requireOneRow(res)
}

// lockCollector locks the collector row until tx ends, serialising run
// starts and finishes of one collector. It returns ErrNotFound if there is no
// such collector.
func lockCollector(tx *gorm.DB, id uuid.UUID) (Collector, error) {
	var c Collector

	err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Take(&c, "id = ?", id).Error

	return c, notFound(err)
}

// StartRun opens a run for the collector with the given ID. An open run whose
// collector has been silent for RunAbandonTimeout is closed as failed first;
// any other open run makes StartRun return ErrRunInProgress. It returns
// ErrNotFound if there is no such collector.
func (st *CollectorStore) StartRun(ctx context.Context, collectorID uuid.UUID, mode RunMode) (CollectorRun, error) {
	now := time.Now().UTC()
	run := CollectorRun{CollectorID: collectorID, Mode: mode, State: RunStateRunning, StartedAt: now}

	err := st.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := lockCollector(tx, collectorID)
		if err != nil {
			return err
		}

		var open []CollectorRun
		if err := tx.Where("collector_id = ? AND state = ?", collectorID, RunStateRunning).Find(&open).Error; err != nil {
			return err
		}

		for _, r := range open {
			if now.Sub(r.lastContact(c.LastSeenAt)) <= RunAbandonTimeout {
				return ErrRunInProgress
			}
		}

		if len(open) > 0 {
			err := tx.Model(&CollectorRun{}).
				Where("collector_id = ? AND state = ?", collectorID, RunStateRunning).
				Updates(map[string]any{"state": RunStateFailed, "finished_at": now, "error": abandonedRunError}).Error
			if err != nil {
				return fmt.Errorf("close abandoned runs: %w", err)
			}
		}

		if err := tx.Create(&run).Error; err != nil {
			return err
		}

		return tx.Model(&Collector{}).Where("id = ?", collectorID).Update("last_seen_at", now).Error
	})
	if err != nil {
		return CollectorRun{}, err
	}

	return run, nil
}

// FinishRun closes the given run of the collector with state, which must be
// RunStateSucceeded or RunStateFailed, recording errMsg (cut to 2000
// characters). A successful full run first deletes the collector's data not
// written in the run; see deleteUnseen. It returns ErrNotFound if the
// collector has no such run and ErrRunNotRunning if the run has finished.
func (st *CollectorStore) FinishRun(
	ctx context.Context,
	collectorID, runID uuid.UUID,
	state RunState,
	errMsg string,
) (CollectorRun, error) {
	now := time.Now().UTC()

	var run CollectorRun

	err := st.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockCollector(tx, collectorID); err != nil {
			return err
		}

		err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Take(&run, "id = ? AND collector_id = ?", runID, collectorID).Error
		if err != nil {
			return notFound(err)
		}

		if run.State != RunStateRunning {
			return ErrRunNotRunning
		}

		if state == RunStateSucceeded && run.Mode == RunModeFull {
			deleted, err := deleteUnseen(tx, collectorID, runID)
			if err != nil {
				return fmt.Errorf("delete data not sent in run: %w", err)
			}

			run.Deleted = deleted
		}

		run.State = state
		run.FinishedAt = &now
		run.Error = truncateRunes(errMsg, maxRunErrorLen)

		err = tx.Model(&run).Select("state", "finished_at", "error", "deleted").Updates(&run).Error
		if err != nil {
			return err
		}

		collectorUpdates := map[string]any{"last_seen_at": now}
		if state == RunStateSucceeded {
			collectorUpdates["last_success_at"] = now
		}

		if err := tx.Model(&Collector{}).Where("id = ?", collectorID).Updates(collectorUpdates).Error; err != nil {
			return err
		}

		return pruneRuns(tx, collectorID)
	})
	if err != nil {
		return CollectorRun{}, err
	}

	return run, nil
}

// deleteUnseen deletes the streets, house numbers and geo objects last
// written by the collector in a run other than runID, and returns how many.
// A street goes only once nothing is left on it, so its cascade never takes
// rows that are not the collector's.
func deleteUnseen(tx *gorm.DB, collectorID, runID uuid.UUID) (int, error) {
	const unseen = "collector_id = ? AND last_run_id IS DISTINCT FROM ?"

	total := 0

	for _, model := range []any{&GeoObject{}, &HouseNumber{}} {
		res := tx.Where(unseen, collectorID, runID).Delete(model)
		if res.Error != nil {
			return 0, res.Error
		}

		total += int(res.RowsAffected)
	}

	res := tx.Where(unseen, collectorID, runID).
		Where("NOT EXISTS (SELECT 1 FROM house_numbers h WHERE h.street_id = streets.id)").
		Where("NOT EXISTS (SELECT 1 FROM geo_objects g WHERE g.street_id = streets.id)").
		Delete(&Street{})
	if res.Error != nil {
		return 0, res.Error
	}

	return total + int(res.RowsAffected), nil
}

// pruneRuns deletes all but the newest maxRunsPerCollector runs of the
// collector.
func pruneRuns(tx *gorm.DB, collectorID uuid.UUID) error {
	return tx.Exec(`DELETE FROM collector_runs WHERE collector_id = ? AND id NOT IN (
		SELECT id FROM collector_runs WHERE collector_id = ? ORDER BY started_at DESC LIMIT ?)`,
		collectorID, collectorID, maxRunsPerCollector).Error
}

// ListRuns returns up to limit runs of the collector with the given ID,
// newest first, or ErrNotFound if there is no such collector.
func (st *CollectorStore) ListRuns(ctx context.Context, collectorID uuid.UUID, limit int) ([]CollectorRun, error) {
	db := st.db.WithContext(ctx)

	if err := db.Select("id").Take(&Collector{}, "id = ?", collectorID).Error; err != nil {
		return nil, notFound(err)
	}

	runs := []CollectorRun{}
	err := db.Where("collector_id = ?", collectorID).Order("started_at DESC").Limit(limit).Find(&runs).Error

	return runs, err
}

// truncateRunes cuts s to at most n runes.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}

	return string([]rune(s)[:n])
}
