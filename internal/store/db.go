// Package store persists streets and house numbers in PostgreSQL.
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
)

// ErrNotFound is returned when the requested row does not exist.
var ErrNotFound = errors.New("not found")

// Open connects to the PostgreSQL database at dsn and verifies the
// connection.
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	return db, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// notFound maps sql.ErrNoRows to ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}

	return err
}

// execUpdate runs an UPDATE statement, mapping a constraint violation
// recognized by isConflict to conflictErr and a zero row count to
// ErrNotFound.
func execUpdate(ctx context.Context, db *sql.DB, query string, args []any, isConflict func(error) bool, conflictErr error) error {
	res, err := db.ExecContext(ctx, query, args...)
	if isConflict(err) {
		return conflictErr
	}

	if err != nil {
		return err
	}

	return requireOneRow(res)
}

// requireOneRow returns ErrNotFound if res affected no rows.
func requireOneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return ErrNotFound
	}

	return nil
}

// queryStrings runs a single-column SELECT. query is always built by callers
// from static SQL fragments with values passed only through args, never
// interpolated into the query text.
func queryStrings(ctx context.Context, db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	values := []string{}

	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}

		values = append(values, v)
	}

	return values, rows.Err()
}
