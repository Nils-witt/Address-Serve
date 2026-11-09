package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GeoObject is a named, grouped point at a street and optionally one of its
// house numbers. Its coordinates are its own and need not match those of the
// street or house number.
type GeoObject struct {
	ID   uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name string    `json:"name" gorm:"not null"`
	// Group is stored as group_name because GROUP is an SQL keyword.
	Group    string    `json:"group" gorm:"column:group_name;not null;index"`
	StreetID uuid.UUID `json:"streetId" gorm:"type:uuid;not null;index"`
	// HouseNumberID, when set, must reference a house number on StreetID.
	HouseNumberID *uuid.UUID `json:"houseNumberId,omitempty" gorm:"type:uuid;index"`
	Latitude      float64    `json:"latitude" gorm:"type:double precision;not null"`
	Longitude     float64    `json:"longitude" gorm:"type:double precision;not null"`
}

// Validate reports whether g has all required fields and valid coordinates.
// Whether the referenced street and house number exist is checked on write.
func (g GeoObject) Validate() error {
	if g.Name == "" || g.Group == "" {
		return errors.New("name and group are required")
	}

	if g.StreetID == uuid.Nil {
		return errors.New("streetId is required")
	}

	if g.HouseNumberID != nil && *g.HouseNumberID == uuid.Nil {
		return errors.New("houseNumberId must be omitted or a valid id")
	}

	if g.Latitude < -90 || g.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}

	if g.Longitude < -180 || g.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}

	return nil
}

var (
	// ErrHouseNumberNotFound is returned when a geo object references a house
	// number that does not exist.
	ErrHouseNumberNotFound = errors.New("referenced house number does not exist")

	// ErrHouseNumberNotOnStreet is returned when a geo object references a
	// house number that belongs to a different street than its streetId.
	ErrHouseNumberNotOnStreet = errors.New("referenced house number is not on the referenced street")
)

// GeoObjectStore reads and writes geo objects.
type GeoObjectStore struct {
	db *gorm.DB
}

// NewGeoObjectStore returns a GeoObjectStore backed by db.
func NewGeoObjectStore(db *gorm.DB) *GeoObjectStore {
	return &GeoObjectStore{db: db}
}

// Create inserts g and returns it with its generated ID. It returns
// ErrStreetNotFound, ErrHouseNumberNotFound or ErrHouseNumberNotOnStreet if
// the references are invalid.
func (st *GeoObjectStore) Create(ctx context.Context, g GeoObject) (GeoObject, error) {
	err := st.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkHouseNumberOnStreet(tx, g); err != nil {
			return err
		}

		err := tx.Create(&g).Error
		if errors.Is(err, gorm.ErrForeignKeyViolated) {
			return ErrStreetNotFound
		}

		return err
	})
	if err != nil {
		return GeoObject{}, err
	}

	return g, nil
}

// GeoObjectFilter narrows List results. Nil and empty fields are ignored.
// Name matches as a substring and Group in full, both case-insensitively.
type GeoObjectFilter struct {
	Name          string
	Group         string
	StreetID      *uuid.UUID
	HouseNumberID *uuid.UUID
}

// List returns the geo objects matching filter, ordered by ID.
func (st *GeoObjectStore) List(ctx context.Context, filter GeoObjectFilter) ([]GeoObject, error) {
	query := st.db.WithContext(ctx)

	if filter.Name != "" {
		query = query.Where("name ILIKE ?", "%"+filter.Name+"%")
	}

	if filter.Group != "" {
		query = query.Where("group_name ILIKE ?", filter.Group)
	}

	if filter.StreetID != nil {
		query = query.Where("street_id = ?", *filter.StreetID)
	}

	if filter.HouseNumberID != nil {
		query = query.Where("house_number_id = ?", *filter.HouseNumberID)
	}

	geoObjects := []GeoObject{}
	err := query.Order("id").Find(&geoObjects).Error

	return geoObjects, err
}

// Get returns the geo object with the given ID, or ErrNotFound.
func (st *GeoObjectStore) Get(ctx context.Context, id uuid.UUID) (GeoObject, error) {
	var g GeoObject

	err := st.db.WithContext(ctx).Take(&g, "id = ?", id).Error

	return g, notFound(err)
}

// Update replaces the geo object with the given ID. It returns ErrNotFound if
// it does not exist, and the same reference errors as Create.
func (st *GeoObjectStore) Update(ctx context.Context, id uuid.UUID, g GeoObject) (GeoObject, error) {
	g.ID = id

	err := st.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkHouseNumberOnStreet(tx, g); err != nil {
			return err
		}

		return updateRow(ctx, tx, &g, gorm.ErrForeignKeyViolated, ErrStreetNotFound)
	})
	if err != nil {
		return GeoObject{}, err
	}

	return g, nil
}

// Delete removes the geo object with the given ID, or returns ErrNotFound.
func (st *GeoObjectStore) Delete(ctx context.Context, id uuid.UUID) error {
	return requireOneRow(st.db.WithContext(ctx).Delete(&GeoObject{}, "id = ?", id))
}

// checkHouseNumberOnStreet verifies that g's house number, if any, exists and
// lies on g's street. It must run inside tx: the row is locked FOR SHARE until
// tx ends, so the house number cannot move to another street before g is
// written.
func checkHouseNumberOnStreet(tx *gorm.DB, g GeoObject) error {
	if g.HouseNumberID == nil {
		return nil
	}

	var h HouseNumber

	err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthShare}).
		Select("street_id").
		Take(&h, "id = ?", *g.HouseNumberID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrHouseNumberNotFound
	}

	if err != nil {
		return err
	}

	if h.StreetID != g.StreetID {
		return ErrHouseNumberNotOnStreet
	}

	return nil
}
