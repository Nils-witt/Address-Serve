package store

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
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
	db *gorm.DB
}

// NewBulkStore returns a BulkStore backed by db.
func NewBulkStore(db *gorm.DB) *BulkStore {
	return &BulkStore{db: db}
}

// insertBatchSize caps the rows per multi-row INSERT, keeping each statement
// well below PostgreSQL's limit of 65535 bind parameters.
const insertBatchSize = 1000

// Create inserts every street and its house numbers in a single transaction:
// either the whole batch is committed, or none of it is.
//
// Rows go in as multi-row INSERTs rather than one per street, so a request of
// 200 streets costs a handful of round trips instead of ~400 (~128ms → ~44ms
// against a local database; far more over a network).
func (bs *BulkStore) Create(ctx context.Context, inputs []BulkStreetInput) ([]BulkResult, error) {
	streets := make([]Street, len(inputs))
	total := 0

	for i, input := range inputs {
		streets[i] = Street{City: input.City, District: input.District, Name: input.Name, Country: input.Country, Latitude: input.Latitude, Longitude: input.Longitude}
		total += len(input.HouseNumbers)
	}

	houseNumbers := make([]HouseNumber, 0, total)

	err := bs.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// RETURNING yields the generated IDs in insertion order, filling in
		// streets[i].ID for each input.
		if err := tx.CreateInBatches(&streets, insertBatchSize).Error; err != nil {
			return err
		}

		for i, input := range inputs {
			for _, h := range input.HouseNumbers {
				houseNumbers = append(houseNumbers, HouseNumber{StreetID: streets[i].ID, Number: h.Number, NumberAddition: h.NumberAddition, Postcode: h.Postcode, Latitude: h.Latitude, Longitude: h.Longitude})
			}
		}

		if len(houseNumbers) == 0 {
			return nil
		}

		if err := tx.CreateInBatches(&houseNumbers, insertBatchSize).Error; err != nil {
			return fmt.Errorf("houseNumbers: %w", err)
		}

		return nil
	})
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return nil, bs.duplicateStreetError(ctx, streets)
	}

	if err != nil {
		return nil, err
	}

	results := make([]BulkResult, len(inputs))
	offset := 0

	for i, input := range inputs {
		end := offset + len(input.HouseNumbers)
		results[i] = BulkResult{Street: streets[i], HouseNumbers: houseNumbers[offset:end:end]}
		offset = end
	}

	return results, nil
}

type streetKey struct {
	name, district, city, country string
}

// duplicateStreetError names the first of streets that violated the unique
// key, either because it is already stored or because it repeats an earlier
// street of the batch. It runs only after a batch failed, so the happy path
// pays no extra round trip for it.
func (bs *BulkStore) duplicateStreetError(ctx context.Context, streets []Street) error {
	keys := make([][]any, len(streets))
	for i, s := range streets {
		keys[i] = []any{s.Name, s.District, s.City, s.Country}
	}

	var stored []Street

	err := bs.db.WithContext(ctx).Select("name", "district", "city", "country").
		Where("(name, district, city, country) IN ?", keys).Find(&stored).Error
	if err != nil {
		return ErrStreetAlreadyExists
	}

	seen := make(map[streetKey]struct{}, len(stored)+len(streets))
	for _, s := range stored {
		seen[streetKey{s.Name, s.District, s.City, s.Country}] = struct{}{}
	}

	for i, s := range streets {
		key := streetKey{s.Name, s.District, s.City, s.Country}
		if _, dup := seen[key]; dup {
			return fmt.Errorf("streets[%d]: %w", i, ErrStreetAlreadyExists)
		}

		seen[key] = struct{}{}
	}

	// The conflicting street was inserted concurrently and has since gone.
	return ErrStreetAlreadyExists
}
