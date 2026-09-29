package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Street is a named street within a district, city and country.
type Street struct {
	ID        uuid.UUID `json:"id"`
	City      string    `json:"city"`
	District  string    `json:"district"`
	Name      string    `json:"name"`
	Country   string    `json:"country"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
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
	db *sql.DB
}

// NewStreetStore returns a StreetStore backed by db.
func NewStreetStore(db *sql.DB) *StreetStore {
	return &StreetStore{db: db}
}

// Create inserts s and returns it with its generated ID.
func (st *StreetStore) Create(ctx context.Context, s Street) (Street, error) {
	err := st.db.QueryRowContext(ctx,
		`INSERT INTO streets (city, district, name, country, latitude, longitude)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		s.City, s.District, s.Name, s.Country, s.Latitude, s.Longitude,
	).Scan(&s.ID)
	if isUniqueViolation(err) {
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
	query := `SELECT id, city, district, name, country, latitude, longitude FROM streets`

	var conditions []string

	var args []any

	if filter.City != "" {
		args = append(args, filter.City)
		conditions = append(conditions, fmt.Sprintf("city ILIKE $%d", len(args)))
	}

	if filter.District != "" {
		args = append(args, filter.District)
		conditions = append(conditions, fmt.Sprintf("district ILIKE $%d", len(args)))
	}

	if filter.Country != "" {
		args = append(args, filter.Country)
		conditions = append(conditions, fmt.Sprintf("country ILIKE $%d", len(args)))
	}

	if filter.Name != "" {
		args = append(args, "%"+filter.Name+"%")
		conditions = append(conditions, fmt.Sprintf("name ILIKE $%d", len(args)))
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ") //nolint:gosec // G202: conditions are static "col ILIKE $N" fragments, values are parameterized via args
	}

	query += " ORDER BY id"

	rows, err := st.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	streets := []Street{}

	for rows.Next() {
		var s Street
		if err := rows.Scan(&s.ID, &s.City, &s.District, &s.Name, &s.Country, &s.Latitude, &s.Longitude); err != nil {
			return nil, err
		}

		streets = append(streets, s)
	}

	return streets, rows.Err()
}

// ListCities returns the distinct city names containing name, or all cities
// if name is empty.
func (st *StreetStore) ListCities(ctx context.Context, name string) ([]string, error) {
	query := `SELECT DISTINCT city FROM streets`

	var args []any
	if name != "" {
		args = append(args, "%"+name+"%")
		query += fmt.Sprintf(" WHERE city ILIKE $%d", len(args))
	}

	query += " ORDER BY city"

	return queryStrings(ctx, st.db, query, args...)
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
	query := `SELECT DISTINCT district, city FROM streets`

	var (
		conditions []string
		args       []any
	)

	if filter.Name != "" {
		args = append(args, "%"+filter.Name+"%")
		conditions = append(conditions, fmt.Sprintf("district ILIKE $%d", len(args)))
	}

	if filter.City != "" {
		args = append(args, filter.City)
		conditions = append(conditions, fmt.Sprintf("city ILIKE $%d", len(args)))
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ") //nolint:gosec // G202: conditions are static "col ILIKE $N" fragments, values are parameterized via args
	}

	query += " ORDER BY district, city"

	rows, err := st.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	districts := []District{}

	for rows.Next() {
		var d District
		if err := rows.Scan(&d.Name, &d.City); err != nil {
			return nil, err
		}

		districts = append(districts, d)
	}

	return districts, rows.Err()
}

// Get returns the street with the given ID, or ErrNotFound.
func (st *StreetStore) Get(ctx context.Context, id uuid.UUID) (Street, error) {
	var s Street

	err := st.db.QueryRowContext(ctx,
		`SELECT id, city, district, name, country, latitude, longitude FROM streets WHERE id = $1`, id,
	).Scan(&s.ID, &s.City, &s.District, &s.Name, &s.Country, &s.Latitude, &s.Longitude)

	return s, notFound(err)
}

// Update replaces the street with the given ID, returning ErrNotFound if it
// does not exist or ErrStreetAlreadyExists on a uniqueness conflict.
func (st *StreetStore) Update(ctx context.Context, id uuid.UUID, s Street) (Street, error) {
	s.ID = id

	err := execUpdate(ctx, st.db,
		`UPDATE streets SET city=$1, district=$2, name=$3, country=$4, latitude=$5, longitude=$6 WHERE id=$7`,
		[]any{s.City, s.District, s.Name, s.Country, s.Latitude, s.Longitude, id},
		isUniqueViolation, ErrStreetAlreadyExists,
	)
	if err != nil {
		return Street{}, err
	}

	return s, nil
}

// Delete removes the street with the given ID and, via cascade, its house
// numbers. It returns ErrNotFound if no such street exists.
func (st *StreetStore) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := st.db.ExecContext(ctx, `DELETE FROM streets WHERE id = $1`, id)
	if err != nil {
		return err
	}

	return requireOneRow(res)
}
