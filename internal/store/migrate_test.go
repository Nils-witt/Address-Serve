package store

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// openIsolatedDB returns a connection whose tables live in a fresh schema of
// the test database, dropped when the test ends. Unlike openBenchDB, it lets
// a test break the schema, for example to plant rows an index would reject.
func openIsolatedDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("ADDRESS_SERV_TEST_DSN")
	if dsn == "" {
		t.Skip("ADDRESS_SERV_TEST_DSN not set")
	}

	// Creates the extensions in public, where the isolated schema's
	// search_path finds them, before any parallel test needs them.
	openBenchDB(t)

	admin, err := Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = Close(admin) })

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}

	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()

	db, err := Open(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = Close(db) })

	return db
}

func TestMigrate_Idempotent(t *testing.T) {
	t.Parallel()

	db := openIsolatedDB(t)

	for range 2 {
		if err := Migrate(t.Context(), db); err != nil {
			t.Fatal(err)
		}
	}

	for _, index := range []string{
		houseNumbersStreetNumberKey, houseNumbersExternalIDKey, streetsExternalIDKey, collectorRunsActiveKey,
	} {
		var exists bool
		if err := db.Raw(indexExistsQuery, index).Scan(&exists).Error; err != nil {
			t.Fatal(err)
		}

		if !exists {
			t.Errorf("index %s missing after Migrate", index)
		}
	}
}

func TestMigrate_ReportsDuplicateHouseNumbers(t *testing.T) {
	t.Parallel()

	db := openIsolatedDB(t)
	if err := Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	// Recreate a database from before the house-number key, holding the
	// duplicates it would reject.
	if err := db.Exec("DROP INDEX " + houseNumbersStreetNumberKey).Error; err != nil {
		t.Fatal(err)
	}

	results, err := NewBulkStore(db).Create(t.Context(), []BulkStreetInput{bulkStreet("Dup Street", 7)})
	if err != nil {
		t.Fatal(err)
	}

	dup := results[0].HouseNumbers[0]
	dup.ID = uuid.Nil

	if err := db.Create(&dup).Error; err != nil {
		t.Fatal(err)
	}

	err = Migrate(t.Context(), db)
	if err == nil || !strings.Contains(err.Error(), "house numbers must be unique per street") ||
		!strings.Contains(err.Error(), "Dup Street 7") {
		t.Fatalf("Migrate = %v, want it to report Dup Street 7", err)
	}

	if err := db.Delete(&HouseNumber{}, "id = ?", dup.ID).Error; err != nil {
		t.Fatal(err)
	}

	if err := Migrate(t.Context(), db); err != nil {
		t.Fatalf("Migrate after removing the duplicate: %v", err)
	}
}

func TestHouseNumberStore_UniquePerStreet(t *testing.T) {
	t.Parallel()

	f := newGeoObjectFixture(t)
	ctx := t.Context()
	existing := f.get(t, f.create(t, GeoObject{Name: "n", Group: "g-" + f.tag, StreetID: f.streetA, HouseNumberID: &f.houseA1, Latitude: 1, Longitude: 1}).ID)

	h, err := f.houseNumbers.Get(ctx, *existing.HouseNumberID)
	if err != nil {
		t.Fatal(err)
	}

	addition := "b"

	tests := []struct {
		name    string
		create  HouseNumber
		wantErr error
	}{
		{name: "same number", create: HouseNumber{StreetID: f.streetA, Number: h.Number, Postcode: "1", Latitude: 1, Longitude: 1}, wantErr: ErrHouseNumberAlreadyExists},
		{name: "same number on another street", create: HouseNumber{StreetID: f.streetB, Number: h.Number + 100, Postcode: "1", Latitude: 1, Longitude: 1}},
		{name: "same number with addition", create: HouseNumber{StreetID: f.streetA, Number: h.Number, NumberAddition: &addition, Postcode: "1", Latitude: 1, Longitude: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := f.houseNumbers.Create(ctx, tt.create)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Create = %v, want %v", err, tt.wantErr)
			}
		})
	}

	// Renumbering onto a taken number conflicts too.
	other, err := f.houseNumbers.Get(ctx, f.houseA2)
	if err != nil {
		t.Fatal(err)
	}

	other.Number = h.Number
	if _, err := f.houseNumbers.Update(ctx, other.ID, other); !errors.Is(err, ErrHouseNumberAlreadyExists) {
		t.Errorf("Update = %v, want %v", err, ErrHouseNumberAlreadyExists)
	}
}

func TestBulkStreetInput_RejectsRepeatedNumber(t *testing.T) {
	t.Parallel()

	in := bulkStreet("Twice", 3, 3)
	if err := in.Validate(); !errors.Is(err, ErrHouseNumberAlreadyExists) {
		t.Errorf("Validate = %v, want %v", err, ErrHouseNumberAlreadyExists)
	}

	addition := "a"
	in.HouseNumbers[1].NumberAddition = &addition

	if err := in.Validate(); err != nil {
		t.Errorf("Validate with distinct additions = %v", err)
	}
}
