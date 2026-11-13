package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// AddressMatch is one result of HouseNumberStore.SearchAddresses: a street,
// and one of its house numbers if the street has any. Its coordinates are the
// house number's, or the street's when there is none.
type AddressMatch struct {
	StreetID       uuid.UUID  `json:"streetId"`
	HouseNumberID  *uuid.UUID `json:"houseNumberId,omitempty"`
	Street         string     `json:"street"`
	HouseNumber    *int       `json:"houseNumber,omitempty"`
	NumberAddition *string    `json:"numberAddition,omitempty"`
	Postcode       *string    `json:"postcode,omitempty"`
	City           string     `json:"city"`
	District       string     `json:"district"`
	Country        string     `json:"country"`
	Latitude       float64    `json:"latitude"`
	Longitude      float64    `json:"longitude"`
}

// AddressQuery is a free-text address search split into its parts.
type AddressQuery struct {
	// Street matches street names as a case-insensitive substring.
	Street string
	// Number, when set, restricts results to house numbers starting with it.
	Number *int
}

// ParseAddressQuery splits q into a street name and an optional trailing
// house number, so "Hauptstraße 12a" searches street "Hauptstraße" for house
// numbers starting with 12. A query that is only a number is a street name.
func ParseAddressQuery(q string) AddressQuery {
	q = strings.TrimSpace(q)

	idx := strings.LastIndexFunc(q, unicode.IsSpace)
	if idx < 0 {
		return AddressQuery{Street: q}
	}

	last := q[idx+1:]
	digits := strings.TrimRightFunc(last, unicode.IsLetter)

	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 {
		return AddressQuery{Street: q}
	}

	return AddressQuery{Street: strings.TrimSpace(q[:idx]), Number: &n}
}

// addressSearchQuery lists streets matching @street with their house
// numbers, filtered to numbers starting with @number when it is not NULL.
const addressSearchQuery = `
SELECT s.id AS street_id, h.id AS house_number_id, s.name AS street, h.number AS house_number,
       h.number_addition, h.postcode, s.city, s.district, s.country,
       COALESCE(h.latitude, s.latitude) AS latitude, COALESCE(h.longitude, s.longitude) AS longitude
FROM streets s
LEFT JOIN house_numbers h ON h.street_id = s.id
WHERE s.name ILIKE @street ESCAPE '\'
  AND (CAST(@number AS text) IS NULL OR CAST(h.number AS text) LIKE CAST(@number AS text) || '%')
ORDER BY s.name, s.city, s.district, h.number NULLS FIRST, h.number_addition NULLS FIRST
LIMIT @limit`

// SearchAddresses returns up to limit addresses matching q, ordered by
// street name, city, district and house number.
func (st *HouseNumberStore) SearchAddresses(ctx context.Context, q AddressQuery, limit int) ([]AddressMatch, error) {
	var number any
	if q.Number != nil {
		number = strconv.Itoa(*q.Number)
	}

	matches := []AddressMatch{}

	err := st.db.WithContext(ctx).Raw(addressSearchQuery,
		sql.Named("street", "%"+escapeLike(q.Street)+"%"),
		sql.Named("number", number),
		sql.Named("limit", limit),
	).Scan(&matches).Error

	return matches, err
}
