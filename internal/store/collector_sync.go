package store

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

// SyncOptions tunes a sync.
type SyncOptions struct {
	// GeoMaxDistanceMeters bounds the fallback that attaches a geo object
	// without a usable address to the nearest house number. 0 disables it.
	GeoMaxDistanceMeters float64
}

// Sync makes the rows the collector owns match its output, in one
// transaction: rows in the output are created or updated, owned rows missing
// from it are deleted, and rows owned by others are never changed. See
// docs/collectors/contract.md for the rules. Problems with single entries
// are reported as issues rather than errors; an error means nothing was
// applied.
func (cs *CollectorStore) Sync(
	ctx context.Context, collectorID string, out contract.Output, opts SyncOptions,
) (SyncStats, []SyncIssue, error) {
	var (
		stats  SyncStats
		issues []SyncIssue
	)

	err := cs.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error

		stats, issues, err = syncTx(tx, collectorID, out, opts)

		return err
	})
	if err != nil {
		return SyncStats{}, nil, err
	}

	return stats, issues, nil
}

// syncTx runs a sync inside tx.
func syncTx(tx *gorm.DB, collectorID string, out contract.Output, opts SyncOptions) (SyncStats, []SyncIssue, error) {
	// Serializes syncs of one collector, should two instances ever run it.
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "collector:"+collectorID).Error; err != nil {
		return SyncStats{}, nil, fmt.Errorf("lock collector: %w", err)
	}

	s := &syncer{tx: tx, collectorID: collectorID, opts: opts, issues: newIssueLog()}
	groups := s.planAddresses(out.Addresses)
	geoObjects := s.planGeoObjects(out.GeoObjects)

	steps := []struct {
		name string
		run  func() error
	}{
		{"streets", func() error { return s.syncStreets(groups) }},
		{"house numbers", func() error { return s.syncHouseNumbers(groups) }},
		{"geo objects", func() error { return s.syncGeoObjects(geoObjects) }},
		{"stale streets", s.removeStaleStreets},
	}

	for _, step := range steps {
		if err := step.run(); err != nil {
			return SyncStats{}, nil, fmt.Errorf("sync %s: %w", step.name, err)
		}
	}

	return s.stats, s.issues.list, nil
}

// syncer holds the state of one sync.
type syncer struct {
	tx          *gorm.DB
	collectorID string
	opts        SyncOptions
	stats       SyncStats
	issues      *issueLog

	// ownedStreets holds the collector's streets, including those created
	// during this sync.
	ownedStreets *streetIndex
	// claimedStreets holds the streets that output entries matched or
	// created; the collector's other streets are stale.
	claimedStreets map[uuid.UUID]bool
	// keptStreets holds the collector's streets that output entries named
	// but couldn't update. They and their house numbers stay as they are.
	keptStreets map[uuid.UUID]bool
}

// issueLog collects issues, keeping at most MaxRunIssues of each kind.
type issueLog struct {
	counts map[IssueKind]int
	list   []SyncIssue
}

func newIssueLog() *issueLog {
	return &issueLog{counts: map[IssueKind]int{}}
}

func (l *issueLog) add(issue SyncIssue) {
	if l.counts[issue.Kind] < MaxRunIssues {
		l.list = append(l.list, issue)
	}

	l.counts[issue.Kind]++
}

// foreign is the SQL condition for rows not owned by the syncing collector,
// manual ones included.
const foreign = "collector_id IS DISTINCT FROM ?"

// lookupBatchSize caps the values of one IN list. Street keys have four
// columns, so a batch stays well below PostgreSQL's 65535 bind parameters.
const lookupBatchSize = 1000

// findIn appends to dst the rows whose column matches one of values and that
// meet cond, if it isn't empty, querying in batches.
func findIn[T any](tx *gorm.DB, dst *[]T, column string, values []any, cond string, args ...any) error {
	for batch := range slices.Chunk(values, lookupBatchSize) {
		var rows []T

		query := tx.Where(column+" IN ?", batch)
		if cond != "" {
			query = query.Where(cond, args...)
		}

		if err := query.Find(&rows).Error; err != nil {
			return err
		}

		*dst = append(*dst, rows...)
	}

	return nil
}

// withForeignRows returns those of ids that rows of table not owned by the
// collector reference in column.
func (s *syncer) withForeignRows(ids []uuid.UUID, table, column string) (map[uuid.UUID]bool, error) {
	held := map[uuid.UUID]bool{}

	for batch := range slices.Chunk(ids, lookupBatchSize) {
		var found []uuid.UUID

		err := s.tx.Table(table).Distinct(column).
			Where(column+" IN ?", batch).Where(foreign, s.collectorID).
			Pluck(column, &found).Error
		if err != nil {
			return nil, err
		}

		for _, id := range found {
			held[id] = true
		}
	}

	return held, nil
}

// release turns the collector's rows of model with the given IDs into manual
// ones. Their external IDs go too: only collectors' rows have one.
func (s *syncer) release(model any, ids []uuid.UUID) error {
	for batch := range slices.Chunk(ids, lookupBatchSize) {
		err := s.tx.Model(model).Where("id IN ?", batch).
			Updates(map[string]any{"collector_id": nil, "external_id": nil}).Error
		if err != nil {
			return err
		}
	}

	return nil
}

// deleteIDs deletes the rows of model with the given IDs and returns how many
// it deleted.
func (s *syncer) deleteIDs(model any, ids []uuid.UUID) (int, error) {
	var deleted int64

	for batch := range slices.Chunk(ids, lookupBatchSize) {
		res := s.tx.Where("id IN ?", batch).Delete(model)
		if res.Error != nil {
			return int(deleted), res.Error
		}

		deleted += res.RowsAffected
	}

	return int(deleted), nil
}

// ownerName describes who owns a row with the given collector ID.
func ownerName(collectorID *string) string {
	if collectorID == nil {
		return "a manually created row"
	}

	return fmt.Sprintf("collector %q", *collectorID)
}

// nearestHouseNumber returns the house number closest to lat and lon if it
// lies within maxMeters.
func nearestHouseNumber(tx *gorm.DB, lat, lon, maxMeters float64) (*HouseNumber, error) {
	var rows []struct {
		ID       uuid.UUID
		StreetID uuid.UUID
		Distance float64
	}

	err := tx.Raw(`SELECT h.id, h.street_id,
			earth_distance(ll_to_earth(h.latitude, h.longitude), ll_to_earth(@lat, @lon)) AS distance
		FROM house_numbers h
		ORDER BY ll_to_earth(h.latitude, h.longitude) <-> ll_to_earth(@lat, @lon)
		LIMIT 1`, sql.Named("lat", lat), sql.Named("lon", lon)).Scan(&rows).Error
	if err != nil || len(rows) == 0 || rows[0].Distance > maxMeters {
		return nil, err
	}

	return &HouseNumber{ID: rows[0].ID, StreetID: rows[0].StreetID}, nil
}
