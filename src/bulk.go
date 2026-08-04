package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type BulkHouseNumberInput struct {
	Number         int     `json:"number"`
	NumberAddition *string `json:"numberAddition,omitempty"`
	Postcode       string  `json:"postcode"`
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
}

func (h BulkHouseNumberInput) validate() error {
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

type BulkStreetInput struct {
	City         string                 `json:"city"`
	District     string                 `json:"district"`
	Name         string                 `json:"name"`
	Latitude     float64                `json:"latitude"`
	Longitude    float64                `json:"longitude"`
	HouseNumbers []BulkHouseNumberInput `json:"houseNumbers"`
}

func (s BulkStreetInput) validate() error {
	street := Street{City: s.City, District: s.District, Name: s.Name, Latitude: s.Latitude, Longitude: s.Longitude}
	if err := street.validate(); err != nil {
		return err
	}
	for i, h := range s.HouseNumbers {
		if err := h.validate(); err != nil {
			return fmt.Errorf("houseNumbers[%d]: %w", i, err)
		}
	}
	return nil
}

type BulkResult struct {
	Street       Street        `json:"street"`
	HouseNumbers []HouseNumber `json:"houseNumbers"`
}

type bulkStore struct {
	db *sql.DB
}

// create inserts every street and its house numbers in a single transaction:
// either the whole batch is committed, or none of it is.
func (bs *bulkStore) create(inputs []BulkStreetInput) ([]BulkResult, error) {
	tx, err := bs.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	results := make([]BulkResult, 0, len(inputs))
	for i, input := range inputs {
		street := Street{City: input.City, District: input.District, Name: input.Name, Latitude: input.Latitude, Longitude: input.Longitude}
		err := tx.QueryRow(
			`INSERT INTO streets (city, district, name, latitude, longitude)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			street.City, street.District, street.Name, street.Latitude, street.Longitude,
		).Scan(&street.ID)
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("streets[%d]: %w", i, errStreetAlreadyExists)
		}
		if err != nil {
			return nil, fmt.Errorf("streets[%d]: %w", i, err)
		}

		houseNumbers := make([]HouseNumber, 0, len(input.HouseNumbers))
		for j, h := range input.HouseNumbers {
			houseNumber := HouseNumber{StreetID: street.ID, Number: h.Number, NumberAddition: h.NumberAddition, Postcode: h.Postcode, Latitude: h.Latitude, Longitude: h.Longitude}
			err := tx.QueryRow(
				`INSERT INTO house_numbers (street_id, number, number_addition, postcode, latitude, longitude)
				 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
				houseNumber.StreetID, houseNumber.Number, houseNumber.NumberAddition, houseNumber.Postcode, houseNumber.Latitude, houseNumber.Longitude,
			).Scan(&houseNumber.ID)
			if err != nil {
				return nil, fmt.Errorf("streets[%d].houseNumbers[%d]: %w", i, j, err)
			}
			houseNumbers = append(houseNumbers, houseNumber)
		}

		results = append(results, BulkResult{Street: street, HouseNumbers: houseNumbers})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func registerBulkRoutes(mux *http.ServeMux, store *bulkStore) {
	mux.HandleFunc("POST /api/bulk", func(w http.ResponseWriter, r *http.Request) {
		var inputs []BulkStreetInput
		if err := json.NewDecoder(r.Body).Decode(&inputs); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if len(inputs) == 0 {
			writeError(w, http.StatusBadRequest, "at least one street is required")
			return
		}
		for i, input := range inputs {
			if err := input.validate(); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("streets[%d]: %s", i, err.Error()))
				return
			}
		}

		results, err := store.create(inputs)
		if errors.Is(err, errStreetAlreadyExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, results)
	})
}
