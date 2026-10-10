package store

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func bulkStreet(name string, houseNumbers ...int) BulkStreetInput {
	in := BulkStreetInput{City: "Testcity", District: "Testdistrict", Name: name, Country: "Germany", Latitude: 50, Longitude: 7}
	for _, n := range houseNumbers {
		in.HouseNumbers = append(in.HouseNumbers, BulkHouseNumberInput{Number: n, Postcode: "53111", Latitude: 50, Longitude: 7})
	}

	return in
}

// createBulk creates inputs and deletes them again when t ends.
func createBulk(t *testing.T, db *gorm.DB, inputs []BulkStreetInput) []BulkResult {
	t.Helper()

	results, err := NewBulkStore(db).Create(t.Context(), inputs)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		for _, r := range results {
			db.Delete(&Street{}, "id = ?", r.Street.ID)
		}
	})

	return results
}

func TestBulkCreate(t *testing.T) {
	t.Parallel()

	db := openBenchDB(t)
	tag := uuid.NewString()

	inputs := []BulkStreetInput{
		bulkStreet("A "+tag, 1, 2),
		bulkStreet("B " + tag),
		bulkStreet("C "+tag, 3, 4, 5),
	}

	results := createBulk(t, db, inputs)
	if len(results) != len(inputs) {
		t.Fatalf("got %d results, want %d", len(results), len(inputs))
	}

	for i, r := range results {
		checkBulkResult(t, db, inputs[i], r)
	}
}

// checkBulkResult verifies that r is what was stored for in: the street ID
// belongs to the street of that name, and its house numbers point at it.
func checkBulkResult(t *testing.T, db *gorm.DB, in BulkStreetInput, r BulkResult) {
	t.Helper()

	var stored Street
	if err := db.Take(&stored, "id = ?", r.Street.ID).Error; err != nil {
		t.Fatalf("%s: stored street: %v", in.Name, err)
	}

	if stored.Name != in.Name {
		t.Errorf("ID %s belongs to street %q, want %q", r.Street.ID, stored.Name, in.Name)
	}

	if len(r.HouseNumbers) != len(in.HouseNumbers) {
		t.Fatalf("%s: got %d house numbers, want %d", in.Name, len(r.HouseNumbers), len(in.HouseNumbers))
	}

	for j, h := range r.HouseNumbers {
		if h.ID == uuid.Nil || h.StreetID != r.Street.ID || h.Number != in.HouseNumbers[j].Number {
			t.Errorf("%s: houseNumbers[%d] = %+v, want number %d on street %s", in.Name, j, h, in.HouseNumbers[j].Number, r.Street.ID)
		}
	}

	var count int64
	if err := db.Model(&HouseNumber{}).Where("street_id = ?", r.Street.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}

	if int(count) != len(in.HouseNumbers) {
		t.Errorf("%s: %d house numbers stored, want %d", in.Name, count, len(in.HouseNumbers))
	}
}

func TestBulkCreateDuplicate(t *testing.T) {
	t.Parallel()

	db := openBenchDB(t)

	tests := []struct {
		name string
		// inputs builds the batch; tag keeps streets apart between cases.
		inputs  func(t *testing.T, tag string) []BulkStreetInput
		wantErr string
	}{
		{
			name: "already stored",
			inputs: func(t *testing.T, tag string) []BulkStreetInput {
				t.Helper()

				existing := bulkStreet("B " + tag)
				createBulk(t, db, []BulkStreetInput{existing})

				return []BulkStreetInput{bulkStreet("A "+tag, 1), existing}
			},
			wantErr: "streets[1]: " + ErrStreetAlreadyExists.Error(),
		},
		{
			name: "repeated in batch",
			inputs: func(_ *testing.T, tag string) []BulkStreetInput {
				return []BulkStreetInput{bulkStreet("A " + tag), bulkStreet("B " + tag), bulkStreet("A " + tag)}
			},
			wantErr: "streets[2]: " + ErrStreetAlreadyExists.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tag := uuid.NewString()

			_, err := NewBulkStore(db).Create(t.Context(), tt.inputs(t, tag))
			if !errors.Is(err, ErrStreetAlreadyExists) || err.Error() != tt.wantErr {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}

			var count int64
			if err := db.Model(&Street{}).Where("name = ?", "A "+tag).Count(&count).Error; err != nil {
				t.Fatal(err)
			}

			if count != 0 {
				t.Errorf("failed batch left %d streets behind, want 0", count)
			}
		})
	}
}
