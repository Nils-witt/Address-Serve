package store

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The benchmarks and store tests run against a real PostgreSQL database,
// seeded with roughly the size of the Bonn import (5,000 streets, 75,000 house
// numbers). They are skipped unless ADDRESS_SERV_TEST_DSN is set. The database
// is wiped first, so never point it at one holding data you want to keep:
//
//	ADDRESS_SERV_TEST_DSN=postgres://bench:bench@localhost:55432/bench \
//	  go test -run '^$' -bench . -benchmem ./internal/store
const (
	benchStreets             = 5000
	benchHouseNumbersPerStrt = 15
	benchDistricts           = 20
)

var (
	benchOnce    sync.Once
	benchDB      *gorm.DB
	errBenchSeed error
	benchStreet  Street
	benchSeq     atomic.Int64
)

func openBenchDB(tb testing.TB) *gorm.DB {
	tb.Helper()

	dsn := os.Getenv("ADDRESS_SERV_TEST_DSN")
	if dsn == "" {
		tb.Skip("ADDRESS_SERV_TEST_DSN not set")
	}

	benchOnce.Do(func() { benchDB, errBenchSeed = seedBenchDB(dsn) })

	if errBenchSeed != nil {
		tb.Fatal(errBenchSeed)
	}

	return benchDB
}

func seedBenchDB(dsn string) (*gorm.DB, error) {
	ctx := context.Background()

	db, err := Open(ctx, dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Exec("DROP TABLE IF EXISTS house_numbers, streets CASCADE").Error; err != nil {
		return nil, err
	}

	if err := Migrate(ctx, db); err != nil {
		return nil, err
	}

	inputs := benchInputs("seed", benchStreets, benchHouseNumbersPerStrt)
	for start := 0; start < len(inputs); start += 200 {
		if _, err := NewBulkStore(db).Create(ctx, inputs[start:min(start+200, len(inputs))]); err != nil {
			return nil, err
		}
	}

	if err := db.Exec("ANALYZE").Error; err != nil {
		return nil, err
	}

	return db, db.Take(&benchStreet).Error
}

var (
	benchStems    = []string{"Haupt", "Berg", "Kirch", "Linden", "Garten", "Wald", "Schul", "Bach", "Markt", "Rosen", "Ahorn", "Birken", "Eichen", "Mühlen", "Rhein", "Sieg", "Burg", "Kloster", "Wiesen", "Feld"}
	benchSuffixes = []string{"straße", "weg", "allee", "platz", "gasse", "ring", "pfad", "ufer"}
)

// benchInputs returns n distinct streets, each with perStreet house numbers.
// tag keeps names from different calls apart.
func benchInputs(tag string, n, perStreet int) []BulkStreetInput {
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // deterministic test data

	inputs := make([]BulkStreetInput, n)
	for i := range inputs {
		hns := make([]BulkHouseNumberInput, perStreet)
		for j := range hns {
			hns[j] = BulkHouseNumberInput{Number: j + 1, Postcode: "531" + strconv.Itoa(10+i%20), Latitude: 50.7, Longitude: 7.1}
		}

		inputs[i] = BulkStreetInput{
			City:         "Bonn",
			District:     fmt.Sprintf("District %02d", i%benchDistricts),
			Name:         fmt.Sprintf("%s%s %s-%d", benchStems[rng.IntN(len(benchStems))], benchSuffixes[rng.IntN(len(benchSuffixes))], tag, i),
			Country:      "Germany",
			Latitude:     50.7,
			Longitude:    7.1,
			HouseNumbers: hns,
		}
	}

	return inputs
}

func BenchmarkStreetList(b *testing.B) {
	st := NewStreetStore(openBenchDB(b))

	cases := []struct {
		name   string
		filter StreetFilter
	}{
		{"all", StreetFilter{}},
		{"name", StreetFilter{Name: "lindenweg"}},
		{"district", StreetFilter{District: "district 07"}},
		{"name+city", StreetFilter{Name: "mühlenring", City: "bonn"}},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := st.List(b.Context(), c.filter); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkListCities(b *testing.B) {
	st := NewStreetStore(openBenchDB(b))

	for b.Loop() {
		if _, err := st.ListCities(b.Context(), "bon"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkListDistricts(b *testing.B) {
	st := NewStreetStore(openBenchDB(b))

	for b.Loop() {
		if _, err := st.ListDistricts(b.Context(), DistrictFilter{Name: "rict 1", City: "bonn"}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHouseNumberListByStreet(b *testing.B) {
	hs := NewHouseNumberStore(openBenchDB(b))
	id := benchStreet.ID

	for b.Loop() {
		if _, err := hs.List(b.Context(), HouseNumberFilter{StreetID: &id}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStreetUpdate(b *testing.B) {
	st := NewStreetStore(openBenchDB(b))
	s := benchStreet

	for b.Loop() {
		if _, err := st.Update(b.Context(), s.ID, s); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHouseNumberCreate(b *testing.B) {
	db := openBenchDB(b)
	hs := NewHouseNumberStore(db)
	h := HouseNumber{StreetID: benchStreet.ID, Number: 999, Postcode: "53111", Latitude: 50.7, Longitude: 7.1}

	b.Cleanup(func() { db.Where("number = 999").Delete(&HouseNumber{}) })

	for b.Loop() {
		if _, err := hs.Create(b.Context(), h); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBulkCreate mirrors one request of import/bonn.py: 200 streets with
// their house numbers.
func BenchmarkBulkCreate(b *testing.B) {
	db := openBenchDB(b)
	bs := NewBulkStore(db)

	for b.Loop() {
		b.StopTimer()

		inputs := benchInputs(fmt.Sprintf("bulk%d", benchSeq.Add(1)), 200, benchHouseNumbersPerStrt)

		b.StartTimer()

		results, err := bs.Create(b.Context(), inputs)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()

		ids := make([]uuid.UUID, len(results))
		for i, r := range results {
			ids[i] = r.Street.ID
		}

		if err := db.Delete(&Street{}, "id IN ?", ids).Error; err != nil {
			b.Fatal(err)
		}

		b.StartTimer()
	}
}
