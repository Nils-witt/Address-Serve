package store

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// NearbyHouseNumber is a house number with its street and its distance from
// a query point.
type NearbyHouseNumber struct {
	HouseNumber HouseNumber `json:"houseNumber"`
	Street      Street      `json:"street"`
	// DistanceMeters is the great-circle distance from the query point.
	DistanceMeters float64 `json:"distanceMeters"`
}

// nearbyRow is one row of the Nearest query; street columns are prefixed
// with s_ because both tables have the same column names.
type nearbyRow struct {
	ID             uuid.UUID
	StreetID       uuid.UUID
	Number         int
	NumberAddition *string
	Postcode       string
	Latitude       float64
	Longitude      float64
	SID            uuid.UUID `gorm:"column:s_id"`
	SCity          string    `gorm:"column:s_city"`
	SDistrict      string    `gorm:"column:s_district"`
	SName          string    `gorm:"column:s_name"`
	SCountry       string    `gorm:"column:s_country"`
	SLatitude      float64   `gorm:"column:s_latitude"`
	SLongitude     float64   `gorm:"column:s_longitude"`
	Distance       float64
}

// nearestQuery orders by <->, the straight-line distance between points on
// the earth's surface, which ranks the same as the great-circle distance and
// is served by house_numbers_earth_idx.
const nearestQuery = `
SELECT h.id, h.street_id, h.number, h.number_addition, h.postcode, h.latitude, h.longitude,
       s.id AS s_id, s.city AS s_city, s.district AS s_district, s.name AS s_name,
       s.country AS s_country, s.latitude AS s_latitude, s.longitude AS s_longitude,
       earth_distance(ll_to_earth(h.latitude, h.longitude), ll_to_earth(@lat, @lon)) AS distance
FROM house_numbers h
JOIN streets s ON s.id = h.street_id
ORDER BY ll_to_earth(h.latitude, h.longitude) <-> ll_to_earth(@lat, @lon)
LIMIT @limit`

// Nearest returns the limit house numbers closest to the point at lat and
// lon, nearest first, each with its street.
func (st *HouseNumberStore) Nearest(ctx context.Context, lat, lon float64, limit int) ([]NearbyHouseNumber, error) {
	var rows []nearbyRow

	err := st.db.WithContext(ctx).Raw(nearestQuery,
		sql.Named("lat", lat), sql.Named("lon", lon), sql.Named("limit", limit),
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	nearby := make([]NearbyHouseNumber, 0, len(rows))
	for _, r := range rows {
		nearby = append(nearby, NearbyHouseNumber{
			HouseNumber: HouseNumber{
				ID:             r.ID,
				StreetID:       r.StreetID,
				Number:         r.Number,
				NumberAddition: r.NumberAddition,
				Postcode:       r.Postcode,
				Latitude:       r.Latitude,
				Longitude:      r.Longitude,
			},
			Street: Street{
				ID:        r.SID,
				City:      r.SCity,
				District:  r.SDistrict,
				Name:      r.SName,
				Country:   r.SCountry,
				Latitude:  r.SLatitude,
				Longitude: r.SLongitude,
			},
			DistanceMeters: r.Distance,
		})
	}

	return nearby, nil
}
