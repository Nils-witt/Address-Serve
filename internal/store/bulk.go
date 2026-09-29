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

// houseNumberBatchSize caps the rows per multi-row INSERT, keeping each
// statement well below PostgreSQL's limit of 65535 bind parameters.
const houseNumberBatchSize = 1000

// Create inserts every street and its house numbers in a single transaction:
// either the whole batch is committed, or none of it is.
func (bs *BulkStore) Create(ctx context.Context, inputs []BulkStreetInput) ([]BulkResult, error) {
	results := make([]BulkResult, 0, len(inputs))

	err := bs.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i, input := range inputs {
			street := Street{City: input.City, District: input.District, Name: input.Name, Country: input.Country, Latitude: input.Latitude, Longitude: input.Longitude}

			err := tx.Create(&street).Error
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return fmt.Errorf("streets[%d]: %w", i, ErrStreetAlreadyExists)
			}

			if err != nil {
				return fmt.Errorf("streets[%d]: %w", i, err)
			}

			houseNumbers := make([]HouseNumber, 0, len(input.HouseNumbers))
			for _, h := range input.HouseNumbers {
				houseNumbers = append(houseNumbers, HouseNumber{StreetID: street.ID, Number: h.Number, NumberAddition: h.NumberAddition, Postcode: h.Postcode, Latitude: h.Latitude, Longitude: h.Longitude})
			}

			if len(houseNumbers) > 0 {
				if err := tx.CreateInBatches(&houseNumbers, houseNumberBatchSize).Error; err != nil {
					return fmt.Errorf("streets[%d].houseNumbers: %w", i, err)
				}
			}

			results = append(results, BulkResult{Street: street, HouseNumbers: houseNumbers})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return results, nil
}
