package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Street is a named street within a district, city and country.
type Street struct {
	ID        uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	City      string    `json:"city" gorm:"not null;uniqueIndex:streets_name_district_city_country_key,priority:3"`
	District  string    `json:"district" gorm:"not null;uniqueIndex:streets_name_district_city_country_key,priority:2"`
	Name      string    `json:"name" gorm:"not null;uniqueIndex:streets_name_district_city_country_key,priority:1"`
	Country   string    `json:"country" gorm:"not null;uniqueIndex:streets_name_district_city_country_key,priority:4"`
	Latitude  float64   `json:"latitude" gorm:"type:double precision;not null"`
	Longitude float64   `json:"longitude" gorm:"type:double precision;not null"`

	// HouseNumbers is never loaded; it only declares the foreign key from
	// house_numbers so deleting a street cascades to its house numbers.
	HouseNumbers []HouseNumber `json:"-" gorm:"constraint:OnDelete:CASCADE"`
}

// Validate reports whether s has all required fields and valid coordinates.
func (s Street) Validate() error {
	if s.City == "" || s.District == "" || s.Name == "" || s.Country == "" {
		return errors.New("city, district, name and country are required")
	}

	if s.Latitude < -90 || s.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}

	if s.Longitude < -180 || s.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}

	return nil
}

// ErrStreetAlreadyExists is returned when a street with the same name,
// district, city and country is already stored.
var ErrStreetAlreadyExists = errors.New("a street with this name, district, city and country already exists")

// StreetStore reads and writes streets.
type StreetStore struct {
	db *gorm.DB
}

// NewStreetStore returns a StreetStore backed by db.
func NewStreetStore(db *gorm.DB) *StreetStore {
	return &StreetStore{db: db}
}

// Create inserts s and returns it with its generated ID.
func (st *StreetStore) Create(ctx context.Context, s Street) (Street, error) {
	err := st.db.WithContext(ctx).Create(&s).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return Street{}, ErrStreetAlreadyExists
	}

	return s, err
}

// StreetFilter narrows List results. Empty fields are ignored; Name matches
// substrings, the others match case-insensitively in full.
type StreetFilter struct {
	City     string
	District string
	Name     string
	Country  string
}

// List returns the streets matching filter, ordered by ID.
func (st *StreetStore) List(ctx context.Context, filter StreetFilter) ([]Street, error) {
	query := st.db.WithContext(ctx)

	if filter.City != "" {
		query = query.Where("city ILIKE ?", filter.City)
	}

	if filter.District != "" {
		query = query.Where("district ILIKE ?", filter.District)
	}

	if filter.Country != "" {
		query = query.Where("country ILIKE ?", filter.Country)
	}

	if filter.Name != "" {
		query = query.Where("name ILIKE ?", "%"+filter.Name+"%")
	}

	streets := []Street{}
	err := query.Order("id").Find(&streets).Error

	return streets, err
}

// ListCities returns the distinct city names containing name, or all cities
// if name is empty.
func (st *StreetStore) ListCities(ctx context.Context, name string) ([]string, error) {
	query := st.db.WithContext(ctx).Model(&Street{})
	if name != "" {
		query = query.Where("city ILIKE ?", "%"+name+"%")
	}

	cities := []string{}
	err := query.Distinct("city").Order("city").Pluck("city", &cities).Error

	return cities, err
}

// DistrictFilter narrows ListDistricts results. Empty fields are ignored.
type DistrictFilter struct {
	Name string
	City string
}

// District is a district together with the city it belongs to.
type District struct {
	Name string `json:"name"`
	City string `json:"city"`
}

// ListDistricts returns the distinct districts matching filter.
func (st *StreetStore) ListDistricts(ctx context.Context, filter DistrictFilter) ([]District, error) {
	query := st.db.WithContext(ctx).Model(&Street{})

	if filter.Name != "" {
		query = query.Where("district ILIKE ?", "%"+filter.Name+"%")
	}

	if filter.City != "" {
		query = query.Where("city ILIKE ?", filter.City)
	}

	districts := []District{}
	err := query.Distinct("district AS name", "city").Order("district, city").Scan(&districts).Error

	return districts, err
}

// Get returns the street with the given ID, or ErrNotFound.
func (st *StreetStore) Get(ctx context.Context, id uuid.UUID) (Street, error) {
	var s Street

	err := st.db.WithContext(ctx).Take(&s, "id = ?", id).Error

	return s, notFound(err)
}

// Update replaces the street with the given ID, returning ErrNotFound if it
// does not exist or ErrStreetAlreadyExists on a uniqueness conflict.
func (st *StreetStore) Update(ctx context.Context, id uuid.UUID, s Street) (Street, error) {
	s.ID = id

	if err := updateRow(ctx, st.db, &s, gorm.ErrDuplicatedKey, ErrStreetAlreadyExists); err != nil {
		return Street{}, err
	}

	return s, nil
}

// Delete removes the street with the given ID and, via cascade, its house
// numbers. It returns ErrNotFound if no such street exists.
func (st *StreetStore) Delete(ctx context.Context, id uuid.UUID) error {
	return requireOneRow(st.db.WithContext(ctx).Delete(&Street{}, "id = ?", id))
}
