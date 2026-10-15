package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// HouseNumber is an addressable number on a street.
type HouseNumber struct {
	ID             uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	StreetID       uuid.UUID `json:"streetId" gorm:"type:uuid;not null;index"`
	Number         int       `json:"number" gorm:"type:integer;not null"`
	NumberAddition *string   `json:"numberAddition,omitempty"`
	Postcode       string    `json:"postcode" gorm:"not null"`
	Latitude       float64   `json:"latitude" gorm:"type:double precision;not null"`
	Longitude      float64   `json:"longitude" gorm:"type:double precision;not null"`
}

// Validate reports whether h has all required fields and valid coordinates.
func (h HouseNumber) Validate() error {
	if h.StreetID == uuid.Nil {
		return errors.New("streetId is required")
	}

	if h.Number <= 0 {
		return errors.New("number must be positive")
	}

	if h.Postcode == "" {
		return errors.New("postcode is required")
	}

	if h.Latitude < -90 || h.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}

	if h.Longitude < -180 || h.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}

	return nil
}

// ErrStreetNotFound is returned when a house number references a street that
// does not exist.
var ErrStreetNotFound = errors.New("referenced street does not exist")

// HouseNumberStore reads and writes house numbers.
type HouseNumberStore struct {
	db *gorm.DB
}

// NewHouseNumberStore returns a HouseNumberStore backed by db.
func NewHouseNumberStore(db *gorm.DB) *HouseNumberStore {
	return &HouseNumberStore{db: db}
}

// Create inserts h and returns it with its generated ID.
func (st *HouseNumberStore) Create(ctx context.Context, h HouseNumber) (HouseNumber, error) {
	err := st.db.WithContext(ctx).Create(&h).Error
	if errors.Is(err, gorm.ErrForeignKeyViolated) {
		return HouseNumber{}, ErrStreetNotFound
	}

	return h, err
}

// HouseNumberFilter narrows List results. Nil and empty fields are ignored.
// Postcode matches as a prefix and StreetName as a substring; City, District
// and Country match in full. All string fields match case-insensitively, and
// those other than Postcode filter by the house number's street.
type HouseNumberFilter struct {
	StreetID   *uuid.UUID
	Number     *int
	Postcode   string
	StreetName string
	City       string
	District   string
	Country    string
}

// hasStreetFields reports whether f filters on columns of the streets table.
func (f HouseNumberFilter) hasStreetFields() bool {
	return f.StreetName != "" || f.City != "" || f.District != "" || f.Country != ""
}

// List returns the house numbers matching filter, ordered by ID.
func (st *HouseNumberStore) List(ctx context.Context, filter HouseNumberFilter) ([]HouseNumber, error) {
	// Columns are qualified because the streets join below also has id
	// columns.
	query := st.db.WithContext(ctx).Model(&HouseNumber{}).Select("house_numbers.*")

	if filter.StreetID != nil {
		query = query.Where("house_numbers.street_id = ?", *filter.StreetID)
	}

	if filter.Number != nil {
		query = query.Where("house_numbers.number = ?", *filter.Number)
	}

	if filter.Postcode != "" {
		query = query.Where("house_numbers.postcode ILIKE ?", filter.Postcode+"%")
	}

	if filter.hasStreetFields() {
		query = query.Joins("JOIN streets ON streets.id = house_numbers.street_id")
	}

	if filter.StreetName != "" {
		query = query.Where("streets.name ILIKE ?", "%"+filter.StreetName+"%")
	}

	if filter.City != "" {
		query = query.Where("streets.city ILIKE ?", filter.City)
	}

	if filter.District != "" {
		query = query.Where("streets.district ILIKE ?", filter.District)
	}

	if filter.Country != "" {
		query = query.Where("streets.country ILIKE ?", filter.Country)
	}

	houseNumbers := []HouseNumber{}
	err := query.Order("house_numbers.id").Find(&houseNumbers).Error

	return houseNumbers, err
}

// Get returns the house number with the given ID, or ErrNotFound.
func (st *HouseNumberStore) Get(ctx context.Context, id uuid.UUID) (HouseNumber, error) {
	var h HouseNumber

	err := st.db.WithContext(ctx).Take(&h, "id = ?", id).Error

	return h, notFound(err)
}

// Update replaces the house number with the given ID, returning ErrNotFound
// if it does not exist or ErrStreetNotFound if StreetID is unknown.
func (st *HouseNumberStore) Update(ctx context.Context, id uuid.UUID, h HouseNumber) (HouseNumber, error) {
	h.ID = id

	if err := updateRow(ctx, st.db, &h, gorm.ErrForeignKeyViolated, ErrStreetNotFound); err != nil {
		return HouseNumber{}, err
	}

	return h, nil
}

// Delete removes the house number with the given ID, or returns ErrNotFound.
func (st *HouseNumberStore) Delete(ctx context.Context, id uuid.UUID) error {
	return requireOneRow(st.db.WithContext(ctx).Delete(&HouseNumber{}, "id = ?", id))
}
