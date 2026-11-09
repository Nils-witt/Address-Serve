package store

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/google/uuid"
)

// earthRadiusMeters is the radius earthdistance assumes.
const earthRadiusMeters = 6378168

// haversine returns the great-circle distance in meters between two points.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)

	return 2 * earthRadiusMeters * math.Asin(math.Sqrt(a))
}

func TestHouseNumberStore_Nearest(t *testing.T) {
	t.Parallel()

	db := openBenchDB(t)
	tag := uuid.NewString()

	// A random point in the southern Pacific, far from the seed data and
	// from every other test, so only the rows created here are near it.
	lat := -65 + 15*rand.Float64()  //nolint:gosec // test data
	lon := -150 + 50*rand.Float64() //nolint:gosec // test data

	// Number 2 is 0.00025° east (~12–18 m at these latitudes) and number 1
	// 0.00018° north (~20 m): 2 is nearer, although comparing raw degrees
	// would rank 1 first. Number 3 is farthest (~56 m).
	offsets := []struct {
		number     int
		dLat, dLon float64
	}{
		{number: 1, dLat: 0.00018},
		{number: 2, dLon: 0.00025},
		{number: 3, dLat: -0.0005},
	}

	in := BulkStreetInput{City: "Reverse " + tag, District: "Mitte", Name: "Nahweg " + tag, Country: "Germany", Latitude: lat, Longitude: lon}
	for _, o := range offsets {
		in.HouseNumbers = append(in.HouseNumbers, BulkHouseNumberInput{
			Number: o.number, Postcode: "53111", Latitude: lat + o.dLat, Longitude: lon + o.dLon,
		})
	}

	street := createBulk(t, db, []BulkStreetInput{in})[0].Street

	tests := []struct {
		name     string
		limit    int
		expected []int // house numbers, nearest first
	}{
		{name: "default returns the nearest one", limit: 1, expected: []int{2}},
		{name: "n nearest in order", limit: 3, expected: []int{2, 1, 3}},
	}

	st := NewHouseNumberStore(db)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := st.Nearest(t.Context(), lat, lon, tt.limit)
			if err != nil {
				t.Fatal(err)
			}

			if len(got) != len(tt.expected) {
				t.Fatalf("got %d results, want %d", len(got), len(tt.expected))
			}

			for i, n := range got {
				if n.HouseNumber.Number != tt.expected[i] {
					t.Errorf("result %d is number %d, want %d", i, n.HouseNumber.Number, tt.expected[i])
				}

				if n.Street.ID != street.ID || n.Street.Name != street.Name || n.HouseNumber.StreetID != street.ID {
					t.Errorf("result %d street = %+v, want %+v", i, n.Street, street)
				}

				want := haversine(lat, lon, n.HouseNumber.Latitude, n.HouseNumber.Longitude)
				if math.Abs(n.DistanceMeters-want) > 0.5 {
					t.Errorf("result %d distance = %.2f m, want %.2f m", i, n.DistanceMeters, want)
				}
			}
		})
	}
}
