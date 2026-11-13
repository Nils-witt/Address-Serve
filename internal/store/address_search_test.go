package store

import (
	"testing"

	"github.com/google/uuid"
)

func TestParseAddressQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		q          string
		wantStreet string
		wantNumber *int
	}{
		{name: "street only", q: "Hauptstraße", wantStreet: "Hauptstraße"},
		{name: "street with number", q: "Hauptstraße 12", wantStreet: "Hauptstraße", wantNumber: new(12)},
		{name: "number with addition", q: "Am Hof 3a", wantStreet: "Am Hof", wantNumber: new(3)},
		{name: "multi word street", q: "Straße des 17. Juni", wantStreet: "Straße des 17. Juni"},
		{name: "surrounding spaces", q: "  Weg 7  ", wantStreet: "Weg", wantNumber: new(7)},
		{name: "only a number", q: "12", wantStreet: "12"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ParseAddressQuery(tt.q)
			if got.Street != tt.wantStreet {
				t.Errorf("Street = %q, want %q", got.Street, tt.wantStreet)
			}

			switch {
			case (got.Number == nil) != (tt.wantNumber == nil):
				t.Errorf("Number = %v, want %v", got.Number, tt.wantNumber)
			case got.Number != nil && *got.Number != *tt.wantNumber:
				t.Errorf("Number = %d, want %d", *got.Number, *tt.wantNumber)
			}
		})
	}
}

func TestHouseNumberStore_SearchAddresses(t *testing.T) {
	t.Parallel()

	db := openBenchDB(t)
	tag := uuid.NewString()
	createBulk(t, db, []BulkStreetInput{
		bulkStreet("Search "+tag, 1, 2, 12),
	})
	houseNumbers := NewHouseNumberStore(db)

	tests := []struct {
		name        string
		q           string
		wantNumbers []int
	}{
		{name: "all numbers of the street", q: "search " + tag, wantNumbers: []int{1, 2, 12}},
		{name: "number prefix", q: "Search " + tag + " 1", wantNumbers: []int{1, 12}},
		{name: "exact number", q: "Search " + tag + " 2", wantNumbers: []int{2}},
		{name: "unknown street", q: "Nowhere " + tag},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			matches, err := houseNumbers.SearchAddresses(t.Context(), ParseAddressQuery(tt.q), 10)
			if err != nil {
				t.Fatal(err)
			}

			got := []int{}

			for _, m := range matches {
				if m.HouseNumber != nil {
					got = append(got, *m.HouseNumber)
				}
			}

			if len(got) != len(tt.wantNumbers) {
				t.Fatalf("numbers = %v, want %v", got, tt.wantNumbers)
			}

			for i := range got {
				if got[i] != tt.wantNumbers[i] {
					t.Errorf("numbers = %v, want %v", got, tt.wantNumbers)
				}
			}
		})
	}
}
