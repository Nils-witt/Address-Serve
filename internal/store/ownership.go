package store

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrOwnedByCollector is matched by the errors of writes refused because
// they would change rows a collector owns: only its runs may do that.
var ErrOwnedByCollector = errors.New("owned by a collector")

// OwnedByCollectorError is a write refused because it would change rows of
// the named collectors.
type OwnedByCollectorError struct {
	CollectorIDs []string
	reason       string
}

func (e *OwnedByCollectorError) Error() string { return e.reason }

// Is makes the error match ErrOwnedByCollector.
func (e *OwnedByCollectorError) Is(target error) bool { return target == ErrOwnedByCollector }

// ownedBy returns an OwnedByCollectorError whose message is format with the
// collectors named in place of its one %s.
func ownedBy(collectorIDs []string, format string) error {
	quoted := make([]string, len(collectorIDs))
	for i, id := range collectorIDs {
		quoted[i] = fmt.Sprintf("%q", id)
	}

	names := "collector " + quoted[0]
	if len(quoted) > 1 {
		names = "collectors " + strings.Join(quoted, ", ")
	}

	return &OwnedByCollectorError{CollectorIDs: collectorIDs, reason: fmt.Sprintf(format, names)}
}

// Conditions that keep API writes off collectors' rows. Writes add them to
// their WHERE clause, so the common case stays a single statement; only a
// write that matched nothing looks up why (see refusal).
const (
	manualRow = "collector_id IS NULL"

	streetWithoutOwnedRows = "NOT EXISTS (SELECT 1 FROM house_numbers h WHERE h.street_id = streets.id AND h.collector_id IS NOT NULL)" +
		" AND NOT EXISTS (SELECT 1 FROM geo_objects g WHERE g.street_id = streets.id AND g.collector_id IS NOT NULL)"

	houseNumberWithoutOwnedGeoObjects = "NOT EXISTS (SELECT 1 FROM geo_objects g " +
		"WHERE g.house_number_id = house_numbers.id AND g.collector_id IS NOT NULL)"
)

// dependants names rows that would change if a row went away.
type dependants struct {
	table, column string
}

// refusal explains why a guarded write of the row of table matching cond
// changed nothing: ErrNotFound if there is no such row, an
// OwnedByCollectorError if a collector owns it or, for a delete, rows in
// deps that it would change. If none of these hold anymore, the row went
// away or changed meanwhile, which also counts as not found.
func refusal(tx *gorm.DB, what string, deps []dependants, table, cond string, args ...any) error {
	var rows []struct {
		ID          uuid.UUID
		CollectorID sql.NullString
	}

	if err := tx.Table(table).Select("id", "collector_id").Where(cond, args...).Limit(1).Scan(&rows).Error; err != nil {
		return err
	}

	if len(rows) == 0 {
		return ErrNotFound
	}

	if rows[0].CollectorID.Valid {
		return ownedBy([]string{rows[0].CollectorID.String}, "the "+what+" belongs to %s; only its runs can change it")
	}

	var owners []string

	for _, d := range deps {
		var found []string

		err := tx.Table(d.table).Distinct("collector_id").
			Where(d.column+" = ? AND collector_id IS NOT NULL", rows[0].ID).Order("collector_id").
			Pluck("collector_id", &found).Error
		if err != nil {
			return err
		}

		for _, id := range found {
			if !slices.Contains(owners, id) {
				owners = append(owners, id)
			}
		}
	}

	if len(owners) > 0 {
		return ownedBy(owners, "rows of %s depend on the "+what+"; deleting it would change them")
	}

	return ErrNotFound
}
