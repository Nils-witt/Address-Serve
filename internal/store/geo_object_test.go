package store

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestGeoObject_Validate(t *testing.T) {
	t.Parallel()

	valid := GeoObject{Name: "Hydrant 1", Group: "hydrants", StreetID: uuid.New(), Latitude: 50, Longitude: 7}
	houseNumberID := uuid.New()

	tests := []struct {
		name      string
		modify    func(g *GeoObject)
		expectErr bool
	}{
		{name: "valid without house number", modify: func(*GeoObject) {}},
		{name: "valid with house number", modify: func(g *GeoObject) { g.HouseNumberID = &houseNumberID }},
		{name: "missing name", modify: func(g *GeoObject) { g.Name = "" }, expectErr: true},
		{name: "missing group", modify: func(g *GeoObject) { g.Group = "" }, expectErr: true},
		{name: "missing street id", modify: func(g *GeoObject) { g.StreetID = uuid.Nil }, expectErr: true},
		{name: "nil uuid house number id", modify: func(g *GeoObject) { g.HouseNumberID = &uuid.Nil }, expectErr: true},
		{name: "latitude too large", modify: func(g *GeoObject) { g.Latitude = 90.1 }, expectErr: true},
		{name: "latitude too small", modify: func(g *GeoObject) { g.Latitude = -90.1 }, expectErr: true},
		{name: "longitude too large", modify: func(g *GeoObject) { g.Longitude = 180.1 }, expectErr: true},
		{name: "longitude too small", modify: func(g *GeoObject) { g.Longitude = -180.1 }, expectErr: true},
		{name: "boundary coordinates", modify: func(g *GeoObject) { g.Latitude, g.Longitude = -90, 180 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := valid
			tt.modify(&g)

			err := g.Validate()
			if (err != nil) != tt.expectErr {
				t.Errorf("Validate() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// geoObjectFixture holds two streets, created for one test, and the IDs of
// their house numbers.
type geoObjectFixture struct {
	db           *gorm.DB
	tag          string
	streetA      uuid.UUID
	streetB      uuid.UUID
	houseA1      uuid.UUID
	houseA2      uuid.UUID
	houseB1      uuid.UUID
	geoObjects   *GeoObjectStore
	houseNumbers *HouseNumberStore
}

func newGeoObjectFixture(t *testing.T) geoObjectFixture {
	t.Helper()

	db := openBenchDB(t)
	tag := uuid.NewString()
	results := createBulk(t, db, []BulkStreetInput{
		bulkStreet("Geo A "+tag, 1, 2),
		bulkStreet("Geo B "+tag, 1),
	})

	return geoObjectFixture{
		db:           db,
		tag:          tag,
		streetA:      results[0].Street.ID,
		streetB:      results[1].Street.ID,
		houseA1:      results[0].HouseNumbers[0].ID,
		houseA2:      results[0].HouseNumbers[1].ID,
		houseB1:      results[1].HouseNumbers[0].ID,
		geoObjects:   NewGeoObjectStore(db),
		houseNumbers: NewHouseNumberStore(db),
	}
}

// create stores g and fails the test on error.
func (f geoObjectFixture) create(t *testing.T, g GeoObject) GeoObject {
	t.Helper()

	created, err := f.geoObjects.Create(t.Context(), g)
	if err != nil {
		t.Fatalf("Create(%+v): %v", g, err)
	}

	return created
}

// get returns the stored geo object with the given ID and fails the test on
// error.
func (f geoObjectFixture) get(t *testing.T, id uuid.UUID) GeoObject {
	t.Helper()

	g, err := f.geoObjects.Get(t.Context(), id)
	if err != nil {
		t.Fatalf("Get(%s): %v", id, err)
	}

	return g
}

func TestGeoObjectStore_CreateAndUpdate(t *testing.T) {
	t.Parallel()

	f := newGeoObjectFixture(t)

	created := f.create(t, GeoObject{Name: "Hydrant", Group: "hydrants-" + f.tag, StreetID: f.streetA, HouseNumberID: &f.houseA1, Latitude: 50.1, Longitude: 7.1})
	if created.ID == uuid.Nil {
		t.Fatal("Create did not set an ID")
	}

	got := f.get(t, created.ID)
	if got.Name != "Hydrant" || got.HouseNumberID == nil || *got.HouseNumberID != f.houseA1 || got.Latitude != 50.1 {
		t.Errorf("Get = %+v, want %+v", got, created)
	}

	// Moving to another street without a house number clears the reference.
	update := GeoObject{Name: "Hydrant moved", Group: created.Group, StreetID: f.streetB, Latitude: 51, Longitude: 8}
	if _, err := f.geoObjects.Update(t.Context(), created.ID, update); err != nil {
		t.Fatal(err)
	}

	got = f.get(t, created.ID)
	if got.Name != update.Name || got.StreetID != f.streetB || got.HouseNumberID != nil || got.Latitude != 51 {
		t.Errorf("after Update, Get = %+v, want %+v", got, update)
	}
}

func TestGeoObjectStore_Delete(t *testing.T) {
	t.Parallel()

	f := newGeoObjectFixture(t)
	ctx := t.Context()
	created := f.create(t, GeoObject{Name: "Bench", Group: "benches-" + f.tag, StreetID: f.streetA})

	if err := f.geoObjects.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := f.geoObjects.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: error = %v, want ErrNotFound", err)
	}

	if err := f.geoObjects.Delete(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete: error = %v, want ErrNotFound", err)
	}

	if _, err := f.geoObjects.Update(ctx, created.ID, created); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update after Delete: error = %v, want ErrNotFound", err)
	}
}

func TestGeoObjectStore_InvalidReferences(t *testing.T) {
	t.Parallel()

	f := newGeoObjectFixture(t)
	unknown := uuid.New()
	existing := f.create(t, GeoObject{Name: "Existing", Group: "g-" + f.tag, StreetID: f.streetA, Latitude: 50, Longitude: 7})

	tests := []struct {
		name     string
		input    GeoObject
		expected error
	}{
		{
			name:     "unknown street",
			input:    GeoObject{Name: "x", Group: "g", StreetID: unknown},
			expected: ErrStreetNotFound,
		},
		{
			name:     "unknown house number",
			input:    GeoObject{Name: "x", Group: "g", StreetID: f.streetA, HouseNumberID: &unknown},
			expected: ErrHouseNumberNotFound,
		},
		{
			name:     "house number on another street",
			input:    GeoObject{Name: "x", Group: "g", StreetID: f.streetA, HouseNumberID: &f.houseB1},
			expected: ErrHouseNumberNotOnStreet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := f.geoObjects.Create(t.Context(), tt.input); !errors.Is(err, tt.expected) {
				t.Errorf("Create: error = %v, want %v", err, tt.expected)
			}

			if _, err := f.geoObjects.Update(t.Context(), existing.ID, tt.input); !errors.Is(err, tt.expected) {
				t.Errorf("Update: error = %v, want %v", err, tt.expected)
			}
		})
	}
}

func TestGeoObjectStore_List(t *testing.T) {
	t.Parallel()

	f := newGeoObjectFixture(t)
	hydrants := "Hydrants " + f.tag
	benches := "Benches " + f.tag

	// Names of the geo objects created below, without the tag suffix.
	const (
		bench        = "Bench"
		hydrantNorth = "Hydrant North"
		hydrantSouth = "Hydrant South"
	)

	f.create(t, GeoObject{Name: hydrantNorth + " " + f.tag, Group: hydrants, StreetID: f.streetA, HouseNumberID: &f.houseA1})
	f.create(t, GeoObject{Name: hydrantSouth + " " + f.tag, Group: hydrants, StreetID: f.streetB})
	f.create(t, GeoObject{Name: bench + " " + f.tag, Group: benches, StreetID: f.streetA, HouseNumberID: &f.houseA2})

	tests := []struct {
		name     string
		filter   GeoObjectFilter
		expected []string // name prefixes before the tag
	}{
		{
			name:     "name substring",
			filter:   GeoObjectFilter{Name: f.tag},
			expected: []string{bench, hydrantNorth, hydrantSouth},
		},
		{
			name:     "group is case-insensitive",
			filter:   GeoObjectFilter{Group: strings.ToUpper(hydrants)},
			expected: []string{hydrantNorth, hydrantSouth},
		},
		{
			name:     "group does not match a substring",
			filter:   GeoObjectFilter{Group: f.tag},
			expected: nil,
		},
		{
			name:     "street",
			filter:   GeoObjectFilter{StreetID: &f.streetA},
			expected: []string{bench, hydrantNorth},
		},
		{
			name:     "house number",
			filter:   GeoObjectFilter{HouseNumberID: &f.houseA1},
			expected: []string{hydrantNorth},
		},
		{
			name:     "group and street combine",
			filter:   GeoObjectFilter{Group: hydrants, StreetID: &f.streetB},
			expected: []string{hydrantSouth},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			list, err := f.geoObjects.List(t.Context(), tt.filter)
			if err != nil {
				t.Fatal(err)
			}

			got := []string{}
			for _, g := range list {
				got = append(got, strings.TrimSuffix(g.Name, " "+f.tag))
			}

			slices.Sort(got)

			if !slices.Equal(got, tt.expected) {
				t.Errorf("List(%+v) = %v, want %v", tt.filter, got, tt.expected)
			}
		})
	}
}

func TestGeoObjectStore_ReferenceDeletion(t *testing.T) {
	t.Parallel()

	f := newGeoObjectFixture(t)
	ctx := t.Context()

	onHouse := f.create(t, GeoObject{Name: "On house", Group: "g-" + f.tag, StreetID: f.streetA, HouseNumberID: &f.houseA1})
	onStreetB := f.create(t, GeoObject{Name: "On street B", Group: "g-" + f.tag, StreetID: f.streetB})

	// Deleting a house number keeps the geo object on its street.
	if err := f.houseNumbers.Delete(ctx, f.houseA1); err != nil {
		t.Fatal(err)
	}

	got, err := f.geoObjects.Get(ctx, onHouse.ID)
	if err != nil {
		t.Fatalf("Get after house number delete: %v", err)
	}

	if got.HouseNumberID != nil || got.StreetID != f.streetA {
		t.Errorf("after house number delete, Get = %+v, want street %s and no house number", got, f.streetA)
	}

	// Deleting a street deletes its geo objects.
	if err := NewStreetStore(f.db).Delete(ctx, f.streetB); err != nil {
		t.Fatal(err)
	}

	if _, err := f.geoObjects.Get(ctx, onStreetB.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after street delete: error = %v, want ErrNotFound", err)
	}
}
