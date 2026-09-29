// Package store persists streets and house numbers in PostgreSQL.
package store

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ErrNotFound is returned when the requested row does not exist.
var ErrNotFound = errors.New("not found")

// Open connects to the PostgreSQL database at dsn and verifies the
// connection.
func Open(ctx context.Context, dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		// Maps PostgreSQL constraint violations to gorm.ErrDuplicatedKey and
		// gorm.ErrForeignKeyViolated.
		TranslateError: true,
		Logger: logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(25)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetConnMaxIdleTime(1 * time.Minute)

	return db, nil
}

// Close closes the connection pool underlying db.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// notFound maps gorm.ErrRecordNotFound to ErrNotFound.
func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}

	return err
}

// updateRow replaces every column of row except its primary key, which must
// already be set. A constraint violation matching conflict is mapped to
// conflictErr and a zero row count to ErrNotFound.
func updateRow(ctx context.Context, db *gorm.DB, row any, conflict, conflictErr error) error {
	res := db.WithContext(ctx).Model(row).Select("*").Omit("id").Updates(row)
	if errors.Is(res.Error, conflict) {
		return conflictErr
	}

	if res.Error != nil {
		return res.Error
	}

	return requireOneRow(res)
}

// requireOneRow returns ErrNotFound if res affected no rows.
func requireOneRow(res *gorm.DB) error {
	if res.Error != nil {
		return res.Error
	}

	if res.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}
