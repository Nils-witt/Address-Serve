package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// BulkHouseNumberInput is a house number to create under a BulkStreetInput.
type BulkHouseNumberInput struct {
	Number         int     `json:"number"`
	NumberAddition *string `json:"numberAddition,omitempty"`
	Postcode       string  `json:"postcode"`
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
}

// Validate reports whether h has all required fields and valid coordinates.
func (h BulkHouseNumberInput) Validate() error {
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

// BulkStreetInput is a street to create together with its house numbers.
type BulkStreetInput struct {
	City         string                 `json:"city"`
	District     string                 `json:"district"`
	Name         string                 `json:"name"`
	Country      string                 `json:"country"`
	Latitude     float64                `json:"latitude"`
	Longitude    float64                `json:"longitude"`
	HouseNumbers []BulkHouseNumberInput `json:"houseNumbers"`
}

// Validate reports whether s and all of its house numbers are valid.
func (s BulkStreetInput) Validate() error {
	street := Street{City: s.City, District: s.District, Name: s.Name, Country: s.Country, Latitude: s.Latitude, Longitude: s.Longitude}
	if err := street.Validate(); err != nil {
		return err
	}

	for i, h := range s.HouseNumbers {
		if err := h.Validate(); err != nil {
			return fmt.Errorf("houseNumbers[%d]: %w", i, err)
		}
	}

	return nil
}

// BulkResult is a created street and its created house numbers.
type BulkResult struct {
	Street       Street        `json:"street"`
	HouseNumbers []HouseNumber `json:"houseNumbers"`
}

// BulkStore creates streets and house numbers in batches.
type BulkStore struct {
	db *sql.DB
}

// NewBulkStore returns a BulkStore backed by db.
func NewBulkStore(db *sql.DB) *BulkStore {
	return &BulkStore{db: db}
}

// Create inserts every street and its house numbers in a single transaction:
// either the whole batch is committed, or none of it is.
func (bs *BulkStore) Create(ctx context.Context, inputs []BulkStreetInput) ([]BulkResult, error) {
	tx, err := bs.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	results := make([]BulkResult, 0, len(inputs))
	for i, input := range inputs {
		street := Street{City: input.City, District: input.District, Name: input.Name, Country: input.Country, Latitude: input.Latitude, Longitude: input.Longitude}

		err := tx.QueryRowContext(ctx,
			`INSERT INTO streets (city, district, name, country, latitude, longitude)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			street.City, street.District, street.Name, street.Country, street.Latitude, street.Longitude,
		).Scan(&street.ID)
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("streets[%d]: %w", i, ErrStreetAlreadyExists)
		}

		if err != nil {
			return nil, fmt.Errorf("streets[%d]: %w", i, err)
		}

		houseNumbers, err := bs.insertHouseNumbers(ctx, tx, street.ID, input.HouseNumbers)
		if err != nil {
			return nil, fmt.Errorf("streets[%d].%w", i, err)
		}

		results = append(results, BulkResult{Street: street, HouseNumbers: houseNumbers})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return results, nil
}

// insertHouseNumbers inserts all house numbers for a street with a single
// multi-row statement instead of one round trip per row. PostgreSQL executes
// a multi-row VALUES list in the order given and returns RETURNING rows in
// that same order, so the Nth scanned row corresponds to inputs[N].
func (bs *BulkStore) insertHouseNumbers(ctx context.Context, tx *sql.Tx, streetID uuid.UUID, inputs []BulkHouseNumberInput) ([]HouseNumber, error) {
	houseNumbers := make([]HouseNumber, 0, len(inputs))
	if len(inputs) == 0 {
		return houseNumbers, nil
	}

	placeholders := make([]string, 0, len(inputs))

	args := make([]any, 0, len(inputs)*6)
	for _, h := range inputs {
		n := len(args)
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)", n+1, n+2, n+3, n+4, n+5, n+6))
		args = append(args, streetID, h.Number, h.NumberAddition, h.Postcode, h.Latitude, h.Longitude)
	}

	//nolint:gosec // G202: placeholders are generated $N markers, values are parameterized via args
	query := `INSERT INTO house_numbers (street_id, number, number_addition, postcode, latitude, longitude)
		 VALUES ` + strings.Join(placeholders, ", ") + ` RETURNING id`

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("houseNumbers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	j := 0
	for rows.Next() {
		h := inputs[j]

		houseNumber := HouseNumber{StreetID: streetID, Number: h.Number, NumberAddition: h.NumberAddition, Postcode: h.Postcode, Latitude: h.Latitude, Longitude: h.Longitude}
		if err := rows.Scan(&houseNumber.ID); err != nil {
			return nil, fmt.Errorf("houseNumbers[%d]: %w", j, err)
		}

		houseNumbers = append(houseNumbers, houseNumber)
		j++
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("houseNumbers: %w", err)
	}

	return houseNumbers, nil
}
