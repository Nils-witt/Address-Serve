package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// HouseNumber is an addressable number on a street.
type HouseNumber struct {
	ID             uuid.UUID `json:"id"`
	StreetID       uuid.UUID `json:"streetId"`
	Number         int       `json:"number"`
	NumberAddition *string   `json:"numberAddition,omitempty"`
	Postcode       string    `json:"postcode"`
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
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
	db *sql.DB
}

// NewHouseNumberStore returns a HouseNumberStore backed by db.
func NewHouseNumberStore(db *sql.DB) *HouseNumberStore {
	return &HouseNumberStore{db: db}
}

// Create inserts h and returns it with its generated ID.
func (st *HouseNumberStore) Create(ctx context.Context, h HouseNumber) (HouseNumber, error) {
	err := st.db.QueryRowContext(ctx,
		`INSERT INTO house_numbers (street_id, number, number_addition, postcode, latitude, longitude)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		h.StreetID, h.Number, h.NumberAddition, h.Postcode, h.Latitude, h.Longitude,
	).Scan(&h.ID)
	if isForeignKeyViolation(err) {
		return HouseNumber{}, ErrStreetNotFound
	}

	return h, err
}

// HouseNumberFilter narrows List results. Nil fields are ignored.
type HouseNumberFilter struct {
	StreetID *uuid.UUID
	Number   *int
}

// List returns the house numbers matching filter, ordered by ID.
func (st *HouseNumberStore) List(ctx context.Context, filter HouseNumberFilter) ([]HouseNumber, error) {
	query := `SELECT id, street_id, number, number_addition, postcode, latitude, longitude FROM house_numbers`

	var (
		conditions []string
		args       []any
	)

	if filter.StreetID != nil {
		args = append(args, *filter.StreetID)
		conditions = append(conditions, fmt.Sprintf("street_id = $%d", len(args)))
	}

	if filter.Number != nil {
		args = append(args, *filter.Number)
		conditions = append(conditions, fmt.Sprintf("number = $%d", len(args)))
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ") //nolint:gosec // G202: conditions are static "col = $N" fragments, values are parameterized via args
	}

	query += ` ORDER BY id`

	rows, err := st.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	houseNumbers := []HouseNumber{}

	for rows.Next() {
		var h HouseNumber
		if err := rows.Scan(&h.ID, &h.StreetID, &h.Number, &h.NumberAddition, &h.Postcode, &h.Latitude, &h.Longitude); err != nil {
			return nil, err
		}

		houseNumbers = append(houseNumbers, h)
	}

	return houseNumbers, rows.Err()
}

// Get returns the house number with the given ID, or ErrNotFound.
func (st *HouseNumberStore) Get(ctx context.Context, id uuid.UUID) (HouseNumber, error) {
	var h HouseNumber

	err := st.db.QueryRowContext(ctx,
		`SELECT id, street_id, number, number_addition, postcode, latitude, longitude FROM house_numbers WHERE id = $1`, id,
	).Scan(&h.ID, &h.StreetID, &h.Number, &h.NumberAddition, &h.Postcode, &h.Latitude, &h.Longitude)

	return h, notFound(err)
}

// Update replaces the house number with the given ID, returning ErrNotFound
// if it does not exist or ErrStreetNotFound if StreetID is unknown.
func (st *HouseNumberStore) Update(ctx context.Context, id uuid.UUID, h HouseNumber) (HouseNumber, error) {
	h.ID = id

	err := execUpdate(ctx, st.db,
		`UPDATE house_numbers SET street_id=$1, number=$2, number_addition=$3, postcode=$4, latitude=$5, longitude=$6 WHERE id=$7`,
		[]any{h.StreetID, h.Number, h.NumberAddition, h.Postcode, h.Latitude, h.Longitude, id},
		isForeignKeyViolation, ErrStreetNotFound,
	)
	if err != nil {
		return HouseNumber{}, err
	}

	return h, nil
}

// Delete removes the house number with the given ID, or returns ErrNotFound.
func (st *HouseNumberStore) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := st.db.ExecContext(ctx, `DELETE FROM house_numbers WHERE id = $1`, id)
	if err != nil {
		return err
	}

	return requireOneRow(res)
}
