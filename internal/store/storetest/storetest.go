// Package storetest provides databases for the tests of packages that use
// the store.
package storetest

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nils-witt/address-serve/internal/store"
)

// OpenIsolated returns a migrated database whose tables live in a fresh
// schema of the database at ADDRESS_SERV_TEST_DSN, dropped when the test
// ends. It skips the test when the variable is unset.
//
// Tests need it when they touch rows they don't own: reloading collectors,
// for one, marks every collector it doesn't find as missing.
func OpenIsolated(tb testing.TB) *gorm.DB {
	tb.Helper()

	dsn := os.Getenv("ADDRESS_SERV_TEST_DSN")
	if dsn == "" {
		tb.Skip("ADDRESS_SERV_TEST_DSN not set")
	}

	admin, err := store.Open(tb.Context(), dsn)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = store.Close(admin) })

	// Creates the extensions in public, where the schema's search_path finds
	// them, unless another test already has.
	if err := store.Migrate(tb.Context(), admin); err != nil {
		tb.Fatal(err)
	}

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })

	u, err := url.Parse(dsn)
	if err != nil {
		tb.Fatal(err)
	}

	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()

	db, err := store.Open(tb.Context(), u.String())
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { _ = store.Close(db) })

	if err := store.Migrate(tb.Context(), db); err != nil {
		tb.Fatal(err)
	}

	return db
}
