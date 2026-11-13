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

// streetColumns are the street columns of a nearest-neighbour query,
// prefixed with s_ because the joined tables share column names. Row structs
// hold them in an exported field tagged embedded: GORM skips unexported
// fields, including anonymously embedded unexported types.
type streetColumns struct {
	SID        uuid.UUID `gorm:"column:s_id"`
	SCity      string    `gorm:"column:s_city"`
	SDistrict  string    `gorm:"column:s_district"`
	SName      string    `gorm:"column:s_name"`
	SCountry   string    `gorm:"column:s_country"`
	SLatitude  float64   `gorm:"column:s_latitude"`
	SLongitude float64   `gorm:"column:s_longitude"`
}

// streetSelect selects streetColumns from a streets table aliased s.
const streetSelect = `s.id AS s_id, s.city AS s_city, s.district AS s_district, s.name AS s_name,
       s.country AS s_country, s.latitude AS s_latitude, s.longitude AS s_longitude`

func (c streetColumns) street() Street {
	return Street{
		ID:        c.SID,
		City:      c.SCity,
		District:  c.SDistrict,
		Name:      c.SName,
		Country:   c.SCountry,
		Latitude:  c.SLatitude,
		Longitude: c.SLongitude,
	}
}

// nearbyRow is one row of the HouseNumberStore.Nearest query.
type nearbyRow struct {
	ID             uuid.UUID
	StreetID       uuid.UUID
	Number         int
	NumberAddition *string
	Postcode       string
	Latitude       float64
	Longitude      float64
	StreetCols     streetColumns `gorm:"embedded"`
	Distance       float64
}

// nearestQuery orders by <->, the straight-line distance between points on
// the earth's surface, which ranks the same as the great-circle distance and
// is served by house_numbers_earth_idx.
const nearestQuery = `
SELECT h.id, h.street_id, h.number, h.number_addition, h.postcode, h.latitude, h.longitude,
       ` + streetSelect + `,
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
			Street:         r.StreetCols.street(),
			DistanceMeters: r.Distance,
		})
	}

	return nearby, nil
}

// NearbyGeoObject is a geo object with its street, its house number if it has
// one, and its distance from a query point.
type NearbyGeoObject struct {
	GeoObject   GeoObject    `json:"geoObject"`
	Street      Street       `json:"street"`
	HouseNumber *HouseNumber `json:"houseNumber,omitempty"`
	// DistanceMeters is the great-circle distance from the query point to the
	// geo object's own coordinates.
	DistanceMeters float64 `json:"distanceMeters"`
}

// nearbyGeoObjectRow is one row of the GeoObjectStore.Nearest query. The
// house number columns are prefixed with h_ and nil when the geo object has
// no house number.
type nearbyGeoObjectRow struct {
	ID              uuid.UUID
	Name            string
	GroupName       string
	StreetID        uuid.UUID
	HouseNumberID   *uuid.UUID
	Latitude        float64
	Longitude       float64
	ExternalID      *string
	HNumber         *int          `gorm:"column:h_number"`
	HNumberAddition *string       `gorm:"column:h_number_addition"`
	HPostcode       *string       `gorm:"column:h_postcode"`
	HLatitude       *float64      `gorm:"column:h_latitude"`
	HLongitude      *float64      `gorm:"column:h_longitude"`
	StreetCols      streetColumns `gorm:"embedded"`
	Distance        float64
}

// houseNumber returns the row's house number, or nil if it has none.
func (r nearbyGeoObjectRow) houseNumber() *HouseNumber {
	if r.HouseNumberID == nil || r.HNumber == nil {
		return nil
	}

	return &HouseNumber{
		ID:             *r.HouseNumberID,
		StreetID:       r.StreetID,
		Number:         *r.HNumber,
		NumberAddition: r.HNumberAddition,
		Postcode:       deref(r.HPostcode),
		Latitude:       deref(r.HLatitude),
		Longitude:      deref(r.HLongitude),
	}
}

// deref returns *p, or the zero value if p is nil.
func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}

	return *p
}

// nearestGeoObjectsQuery ranks like nearestQuery and is served by
// geo_objects_earth_idx. A geo object's house number is on its own street, so
// joining it adds no rows.
const nearestGeoObjectsQuery = `
SELECT g.id, g.name, g.group_name, g.street_id, g.house_number_id, g.latitude, g.longitude, g.external_id,
       h.number AS h_number, h.number_addition AS h_number_addition, h.postcode AS h_postcode,
       h.latitude AS h_latitude, h.longitude AS h_longitude,
       ` + streetSelect + `,
       earth_distance(ll_to_earth(g.latitude, g.longitude), ll_to_earth(@lat, @lon)) AS distance
FROM geo_objects g
JOIN streets s ON s.id = g.street_id
LEFT JOIN house_numbers h ON h.id = g.house_number_id
ORDER BY ll_to_earth(g.latitude, g.longitude) <-> ll_to_earth(@lat, @lon)
LIMIT @limit`

// Nearest returns the limit geo objects closest to the point at lat and lon,
// nearest first, each with its street and house number.
func (st *GeoObjectStore) Nearest(ctx context.Context, lat, lon float64, limit int) ([]NearbyGeoObject, error) {
	var rows []nearbyGeoObjectRow

	err := st.db.WithContext(ctx).Raw(nearestGeoObjectsQuery,
		sql.Named("lat", lat), sql.Named("lon", lon), sql.Named("limit", limit),
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	nearby := make([]NearbyGeoObject, 0, len(rows))
	for _, r := range rows {
		nearby = append(nearby, NearbyGeoObject{
			GeoObject: GeoObject{
				ID:            r.ID,
				Name:          r.Name,
				Group:         r.GroupName,
				StreetID:      r.StreetID,
				HouseNumberID: r.HouseNumberID,
				Latitude:      r.Latitude,
				Longitude:     r.Longitude,
				ExternalID:    r.ExternalID,
			},
			Street:         r.StreetCols.street(),
			HouseNumber:    r.houseNumber(),
			DistanceMeters: r.Distance,
		})
	}

	return nearby, nil
}
