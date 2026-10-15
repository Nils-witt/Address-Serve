package store

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestHouseNumberStore_List(t *testing.T) {
	t.Parallel()

	db := openBenchDB(t)
	tag := uuid.NewString()

	// Two streets in different cities; the tag keeps other rows in the
	// shared database out of every filter below.
	north := bulkStreet("Nordstraße "+tag, 1, 2)
	north.City = "Nordstadt " + tag
	north.District = "Mitte"

	south := bulkStreet("Südweg "+tag, 1, 3)
	south.City = "Südstadt " + tag
	south.District = "Rand"
	south.Country = "Austria"

	for i := range south.HouseNumbers {
		south.HouseNumbers[i].Postcode = "53229"
	}

	results := createBulk(t, db, []BulkStreetInput{north, south})

	// Street label and number of each house number created above.
	const (
		north1 = "Nord 1"
		north2 = "Nord 2"
		south1 = "Süd 1"
		south3 = "Süd 3"
	)

	northID := results[0].Street.ID
	one := 1

	tests := []struct {
		name     string
		filter   HouseNumberFilter
		expected []string // street name prefix and number, e.g. "Nord 1"
	}{
		{
			name:     "street name substring",
			filter:   HouseNumberFilter{StreetName: tag},
			expected: []string{north1, north2, south1, south3},
		},
		{
			name:     "street name and city are case-insensitive",
			filter:   HouseNumberFilter{StreetName: "NORD", City: "NORDSTADT " + strings.ToUpper(tag)},
			expected: []string{north1, north2},
		},
		{
			name:     "city matches in full",
			filter:   HouseNumberFilter{City: "südstadt " + tag},
			expected: []string{south1, south3},
		},
		{
			name:     "city does not match a substring",
			filter:   HouseNumberFilter{City: tag},
			expected: nil,
		},
		{
			name:     "district and country",
			filter:   HouseNumberFilter{StreetName: tag, District: "rand", Country: "austria"},
			expected: []string{south1, south3},
		},
		{
			name:     "postcode prefix",
			filter:   HouseNumberFilter{StreetName: tag, Postcode: "532"},
			expected: []string{south1, south3},
		},
		{
			name:     "number across streets",
			filter:   HouseNumberFilter{StreetName: tag, Number: &one},
			expected: []string{north1, south1},
		},
		{
			name:     "street id with number",
			filter:   HouseNumberFilter{StreetID: &northID, Number: &one},
			expected: []string{north1},
		},
	}

	streetLabels := map[uuid.UUID]string{
		results[0].Street.ID: "Nord",
		results[1].Street.ID: "Süd",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			list, err := NewHouseNumberStore(db).List(t.Context(), tt.filter)
			if err != nil {
				t.Fatal(err)
			}

			got := []string{}

			for _, h := range list {
				label, ok := streetLabels[h.StreetID]
				if !ok {
					t.Fatalf("house number %s is on an unexpected street %s", h.ID, h.StreetID)
				}

				got = append(got, label+" "+strconv.Itoa(h.Number))
			}

			slices.Sort(got)

			if !slices.Equal(got, tt.expected) {
				t.Errorf("List(%+v) = %v, want %v", tt.filter, got, tt.expected)
			}
		})
	}
}
