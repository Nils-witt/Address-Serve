package store

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// interruptedRunError is the error of runs that were still running when the
// app stopped.
const interruptedRunError = "interrupted by restart"

// CreateRun records a new running run of the collector. It returns
// ErrNotFound for an unknown collector and ErrRunActive if one is already
// running.
func (cs *CollectorStore) CreateRun(ctx context.Context, collectorID, triggeredBy string) (CollectorRun, error) {
	phase := PhaseCollecting
	run := CollectorRun{
		CollectorID: collectorID,
		Status:      RunRunning,
		Phase:       &phase,
		TriggeredBy: triggeredBy,
		StartedAt:   time.Now(),
	}

	// The partial unique index on running runs settles races between two
	// starts, including from another instance.
	err := cs.db.WithContext(ctx).Create(&run).Error

	switch {
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return CollectorRun{}, ErrRunActive
	case errors.Is(err, gorm.ErrForeignKeyViolated):
		return CollectorRun{}, ErrNotFound
	case err != nil:
		return CollectorRun{}, err
	}

	return run, nil
}

// SetRunPhase records that the running run with the given ID moved to phase.
func (cs *CollectorStore) SetRunPhase(ctx context.Context, id uuid.UUID, phase RunPhase) error {
	return requireOneRow(cs.db.WithContext(ctx).Model(&CollectorRun{}).
		Where("id = ? AND status = ?", id, RunRunning).Update("phase", phase))
}

// RunOutcome is how a run ended.
type RunOutcome struct {
	Status   RunStatus
	ExitCode *int
	Error    *string
	Stats    *SyncStats
	Issues   []SyncIssue
	Log      string
}

// FinishRun records the outcome of the running run with the given ID, then
// deletes the collector's oldest finished runs beyond keep.
func (cs *CollectorStore) FinishRun(ctx context.Context, id uuid.UUID, outcome RunOutcome, keep int) (CollectorRun, error) {
	var run CollectorRun

	err := cs.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&CollectorRun{}).Where("id = ? AND status = ?", id, RunRunning).Updates(map[string]any{
			columnStatus:  outcome.Status,
			"phase":       nil,
			"finished_at": time.Now(),
			"exit_code":   outcome.ExitCode,
			columnError:   outcome.Error,
			"stats":       jsonValue{outcome.Stats},
			"issues":      jsonValue{outcome.Issues},
			"log":         outcome.Log,
		})
		if err := requireOneRow(res); err != nil {
			return err
		}

		if err := tx.Take(&run, "id = ?", id).Error; err != nil {
			return err
		}

		return pruneRuns(tx, run.CollectorID, keep)
	})
	if err != nil {
		return CollectorRun{}, err
	}

	return run, nil
}

// pruneRuns deletes the collector's finished runs beyond the keep newest.
func pruneRuns(tx *gorm.DB, collectorID string, keep int) error {
	if keep <= 0 {
		return nil
	}

	return tx.Exec(`DELETE FROM collector_runs WHERE id IN (
		SELECT id FROM collector_runs WHERE collector_id = ? AND status <> ?
		ORDER BY started_at DESC OFFSET ?)`, collectorID, RunRunning, keep).Error
}

// GetRun returns the run with the given ID, or ErrNotFound.
func (cs *CollectorStore) GetRun(ctx context.Context, id uuid.UUID) (CollectorRun, error) {
	var run CollectorRun

	err := cs.db.WithContext(ctx).Take(&run, "id = ?", id).Error

	return run, notFound(err)
}

// ListRuns returns the collector's runs, newest first, without their issues
// and logs, which only GetRun loads.
func (cs *CollectorStore) ListRuns(ctx context.Context, collectorID string, limit, offset int) ([]CollectorRun, error) {
	runs := []CollectorRun{}
	err := cs.db.WithContext(ctx).Omit("issues", "log").
		Where("collector_id = ?", collectorID).
		Order("started_at DESC, id").Limit(limit).Offset(offset).Find(&runs).Error

	return runs, err
}

// LastRuns returns the newest run of each collector that has one, by
// collector ID, without issues and logs.
func (cs *CollectorStore) LastRuns(ctx context.Context) (map[string]CollectorRun, error) {
	var runs []CollectorRun

	err := cs.db.WithContext(ctx).Raw(`SELECT DISTINCT ON (collector_id)
			id, collector_id, status, phase, triggered_by, started_at, finished_at, exit_code, error, stats
		FROM collector_runs ORDER BY collector_id, started_at DESC`).Scan(&runs).Error
	if err != nil {
		return nil, err
	}

	last := make(map[string]CollectorRun, len(runs))
	for _, r := range runs {
		last[r.CollectorID] = r
	}

	return last, nil
}

// FailInterruptedRuns marks every running run as failed. The app calls it at
// startup, before it starts runs of its own: a run still marked running then
// belonged to a process that stopped, killing its collector with it.
func (cs *CollectorStore) FailInterruptedRuns(ctx context.Context) (int64, error) {
	res := cs.db.WithContext(ctx).Model(&CollectorRun{}).Where("status = ?", RunRunning).
		Updates(map[string]any{
			columnStatus:  RunFailed,
			"phase":       nil,
			"finished_at": time.Now(),
			columnError:   interruptedRunError,
		})

	return res.RowsAffected, res.Error
}

// jsonValue stores v as JSON in a jsonb column, or SQL NULL if v marshals to
// null. Updates with a map skip the serializer tags of the model, so values
// of serialized columns must be encoded explicitly.
type jsonValue struct{ v any }

// Value implements driver.Valuer.
func (j jsonValue) Value() (driver.Value, error) {
	b, err := json.Marshal(j.v)
	if err != nil || string(b) == "null" {
		return nil, err
	}

	return string(b), nil
}
