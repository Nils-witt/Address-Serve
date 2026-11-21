package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

// ownershipFixture holds rows of a collector and manual rows that depend on
// them or that they depend on.
type ownershipFixture struct {
	*syncFixture

	owned       Street
	ownedNumber HouseNumber
	ownedGeo    GeoObject
	// manual holds a geo object of the collector; manualNumber is the
	// target of another.
	manual       Street
	manualNumber HouseNumber
}

const ownedGeoName = "owned geo object"

func newOwnershipFixture(t *testing.T) ownershipFixture {
	t.Helper()

	f := ownershipFixture{syncFixture: newSyncFixture(t)}
	ctx := t.Context()

	manual := f.addr("Manual", 1, 0)

	var err error

	f.manual, err = NewStreetStore(f.db).Create(ctx, Street{
		Name: manual.Street, District: manual.District, City: manual.City, Country: manual.Country,
		Latitude: f.lat, Longitude: f.lon,
	})
	if err != nil {
		t.Fatal(err)
	}

	f.manualNumber, err = NewHouseNumberStore(f.db).Create(ctx, HouseNumber{
		StreetID: f.manual.ID, Number: 1, Postcode: "1", Latitude: f.lat, Longitude: f.lon,
	})
	if err != nil {
		t.Fatal(err)
	}

	onManual := f.geo("hydrant", "on manual", 0)
	onManual.Address = f.ref("Manual", new(1))
	owned := f.geo("hydrant", ownedGeoName, 0)
	owned.ExternalID = new("owned-" + f.id)
	owned.Address = f.ref("Owned", nil)

	f.sync(t, contract.Output{
		Addresses:  []contract.Address{f.addr("Owned", 1, 0)},
		GeoObjects: []contract.GeoObject{onManual, owned},
	})

	f.owned = f.street(t, "Owned")
	f.ownedNumber = f.houseNumbers(t, "Owned")[0]

	for _, g := range f.geoObjects(t) {
		if g.Name == ownedGeoName {
			f.ownedGeo = g
		}
	}

	return f
}

// geoOwned is how refusals to change a collector's geo object start.
const geoOwned = "the geo object belongs to"

func TestOwnership_RefusesChangesToCollectorRows(t *testing.T) {
	t.Parallel()

	f := newOwnershipFixture(t)
	streets, numbers, geo := NewStreetStore(f.db), NewHouseNumberStore(f.db), NewGeoObjectStore(f.db)

	tests := []struct {
		name   string
		write  func() error
		reason string
	}{
		{"update owned street", func() error { _, err := streets.Update(t.Context(), f.owned.ID, f.owned); return err }, "the street belongs to"},
		{"delete owned street", func() error { return streets.Delete(t.Context(), f.owned.ID) }, "the street belongs to"},
		{"update owned house number", func() error {
			_, err := numbers.Update(t.Context(), f.ownedNumber.ID, f.ownedNumber)
			return err
		}, "the house number belongs to"},
		{"delete owned house number", func() error { return numbers.Delete(t.Context(), f.ownedNumber.ID) }, "the house number belongs to"},
		{"update owned geo object", func() error { _, err := geo.Update(t.Context(), f.ownedGeo.ID, f.ownedGeo); return err }, geoOwned},
		{"delete owned geo object", func() error { return geo.Delete(t.Context(), f.ownedGeo.ID) }, geoOwned},
		{"delete owned geo object by external ID", func() error {
			return geo.DeleteByExternalID(t.Context(), *f.ownedGeo.ExternalID)
		}, geoOwned},
		{"upsert owned geo object", func() error {
			_, _, err := geo.UpsertByExternalID(t.Context(), *f.ownedGeo.ExternalID, f.ownedGeo)
			return err
		}, geoOwned},
		{"bulk delete including an owned geo object", func() error {
			_, err := geo.DeleteMany(t.Context(), GeoObjectRefs{IDs: []uuid.UUID{f.ownedGeo.ID}, ExternalIDs: []string{"none"}})
			return err
		}, "some of the geo objects belong to"},
		{"delete manual street holding owned rows", func() error { return streets.Delete(t.Context(), f.manual.ID) }, "depend on the street"},
		{"delete manual house number owned rows point to", func() error {
			return numbers.Delete(t.Context(), f.manualNumber.ID)
		}, "depend on the house number"},
	}

	// Nothing below may change anything; checked once every write was tried.
	t.Cleanup(func() {
		if got := len(f.geoObjects(t)); got != 2 {
			t.Errorf("%d geo objects left, want 2", got)
		}

		if got := f.street(t, "Owned"); got.CollectorID == nil {
			t.Error("the owned street lost its collector")
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.write()

			var owned *OwnedByCollectorError
			if !errors.Is(err, ErrOwnedByCollector) || !errors.As(err, &owned) || owned.CollectorIDs[0] != f.id ||
				!strings.Contains(err.Error(), tt.reason) || !strings.Contains(err.Error(), f.id) {
				t.Errorf("err = %v, want ownership by %s mentioning %q", err, f.id, tt.reason)
			}
		})
	}
}

func TestOwnership_AllowsManualWrites(t *testing.T) {
	t.Parallel()

	f := newOwnershipFixture(t)
	ctx := t.Context()
	streets, numbers := NewStreetStore(f.db), NewHouseNumberStore(f.db)

	// Adding a manual house number to a collector's street.
	added, err := numbers.Create(ctx, HouseNumber{
		StreetID: f.owned.ID, Number: 7, Postcode: "1", Latitude: f.lat, Longitude: f.lon,
		CollectorID: &f.id, ExternalID: new("forged"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if added.CollectorID != nil || added.ExternalID != nil {
		t.Errorf("created house number = %+v, want collector columns ignored", added)
	}

	// Changing a manual street that a collector's geo object is on.
	f.manual.Name = "Manual renamed"
	if _, err := streets.Update(ctx, f.manual.ID, f.manual); err != nil {
		t.Errorf("rename manual street: %v", err)
	}

	// Deleting rows that don't exist is still not found.
	if err := streets.Delete(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete unknown street: err = %v, want ErrNotFound", err)
	}

	if err := numbers.Delete(ctx, added.ID); err != nil {
		t.Errorf("delete the manual house number again: %v", err)
	}
}
