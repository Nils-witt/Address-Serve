package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GeoObject is a named, grouped point at a street and optionally one of its
// house numbers. Its coordinates are its own and need not match those of the
// street or house number. Its name is unique within its group.
type GeoObject struct {
	ID   uuid.UUID `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name string    `json:"name" gorm:"not null"`
	// Group is stored as group_name because GROUP is an SQL keyword.
	Group    string    `json:"group" gorm:"column:group_name;not null;index"`
	StreetID uuid.UUID `json:"streetId" gorm:"type:uuid;not null;index"`
	// HouseNumberID, when set, must reference a house number on StreetID.
	HouseNumberID *uuid.UUID `json:"houseNumberId,omitempty" gorm:"type:uuid;index"`
	Latitude      float64    `json:"latitude" gorm:"type:double precision;not null"`
	Longitude     float64    `json:"longitude" gorm:"type:double precision;not null"`
	// ExternalID, when set, identifies the object in another system that
	// keeps it in sync, such as a tileserve-go instance. It is unique.
	ExternalID *string `json:"externalId,omitempty" gorm:"column:external_id"`
	// CollectorID, when set, names the collector that owns the geo object;
	// only that collector's runs may change it.
	CollectorID *string `json:"collectorId,omitempty" gorm:"index"`
}

// maxExternalIDLength bounds GeoObject.ExternalID.
const maxExternalIDLength = 255

// Validate reports whether g has all required fields and valid coordinates.
// Whether the referenced street and house number exist is checked on write.
func (g GeoObject) Validate() error {
	if g.Name == "" || g.Group == "" {
		return errors.New("name and group are required")
	}

	if g.StreetID == uuid.Nil {
		return errors.New("streetId is required")
	}

	if g.HouseNumberID != nil && *g.HouseNumberID == uuid.Nil {
		return errors.New("houseNumberId must be omitted or a valid id")
	}

	if g.Latitude < -90 || g.Latitude > 90 {
		return errors.New("latitude must be between -90 and 90")
	}

	if g.Longitude < -180 || g.Longitude > 180 {
		return errors.New("longitude must be between -180 and 180")
	}

	return g.ValidateExternalID()
}

// ValidateExternalID reports whether g's ExternalID is nil or 1 to 255 bytes
// long.
func (g GeoObject) ValidateExternalID() error {
	if g.ExternalID != nil && (*g.ExternalID == "" || len(*g.ExternalID) > maxExternalIDLength) {
		return errors.New("externalId must be omitted or 1 to 255 characters long")
	}

	return nil
}

var (
	// ErrHouseNumberNotFound is returned when a geo object references a house
	// number that does not exist.
	ErrHouseNumberNotFound = errors.New("referenced house number does not exist")

	// ErrHouseNumberNotOnStreet is returned when a geo object references a
	// house number that belongs to a different street than its streetId.
	ErrHouseNumberNotOnStreet = errors.New("referenced house number is not on the referenced street")

	// ErrExternalIDTaken is returned when a geo object's external ID is
	// already used by another geo object.
	ErrExternalIDTaken = errors.New("externalId is already used by another geo object")

	// ErrNameTaken is returned when another geo object in the same group
	// already has the geo object's name.
	ErrNameTaken = errors.New("name is already used by another geo object in this group")
)

// Names of the unique indexes on geo_objects, created by Migrate.
const (
	geoObjectsExternalIDKey = "geo_objects_external_id_key"
	geoObjectsGroupNameKey  = "geo_objects_group_name_name_key"
)

// geoObjectWriteError maps a constraint violation from writing a geo object
// to the matching domain error and returns other errors unchanged.
func geoObjectWriteError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrForeignKeyViolated):
		return ErrStreetNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == geoObjectsGroupNameKey {
			return ErrNameTaken
		}

		return ErrExternalIDTaken
	}

	return err
}

// GeoObjectStore reads and writes geo objects.
type GeoObjectStore struct {
	db *gorm.DB
}

// NewGeoObjectStore returns a GeoObjectStore backed by db.
func NewGeoObjectStore(db *gorm.DB) *GeoObjectStore {
	return &GeoObjectStore{db: db}
}

// Create inserts g and returns it with its generated ID. It returns
// ErrStreetNotFound, ErrHouseNumberNotFound or ErrHouseNumberNotOnStreet if
// the references are invalid, and ErrNameTaken or ErrExternalIDTaken if
// another geo object already has g's name in its group or g's external ID.
func (st *GeoObjectStore) Create(ctx context.Context, g GeoObject) (GeoObject, error) {
	g.CollectorID = nil

	err := st.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkHouseNumberOnStreet(tx, g); err != nil {
			return err
		}

		return geoObjectWriteError(tx.Create(&g).Error)
	})
	if err != nil {
		return GeoObject{}, err
	}

	return g, nil
}

// escapeLike escapes the LIKE wildcards in s, using backslash as the escape
// character.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
}

// GeoObjectFilter narrows List results. Nil and empty fields are ignored.
// Name matches as a substring and Group in full, both case-insensitively.
// ExternalID matches in full and ExternalIDPrefix as a prefix, both
// case-sensitively.
type GeoObjectFilter struct {
	Name             string
	Group            string
	StreetID         *uuid.UUID
	HouseNumberID    *uuid.UUID
	ExternalID       string
	ExternalIDPrefix string
}

// List returns the geo objects matching filter, ordered by ID.
func (st *GeoObjectStore) List(ctx context.Context, filter GeoObjectFilter) ([]GeoObject, error) {
	query := st.db.WithContext(ctx)

	if filter.Name != "" {
		query = query.Where("name ILIKE ?", "%"+filter.Name+"%")
	}

	if filter.Group != "" {
		query = query.Where("group_name ILIKE ?", filter.Group)
	}

	if filter.StreetID != nil {
		query = query.Where("street_id = ?", *filter.StreetID)
	}

	if filter.HouseNumberID != nil {
		query = query.Where("house_number_id = ?", *filter.HouseNumberID)
	}

	if filter.ExternalID != "" {
		query = query.Where("external_id = ?", filter.ExternalID)
	}

	if filter.ExternalIDPrefix != "" {
		query = query.Where(`external_id LIKE ? ESCAPE '\'`, escapeLike(filter.ExternalIDPrefix)+"%")
	}

	geoObjects := []GeoObject{}
	err := query.Order("id").Find(&geoObjects).Error

	return geoObjects, err
}

// Get returns the geo object with the given ID, or ErrNotFound.
func (st *GeoObjectStore) Get(ctx context.Context, id uuid.UUID) (GeoObject, error) {
	var g GeoObject

	err := st.db.WithContext(ctx).Take(&g, "id = ?", id).Error

	return g, notFound(err)
}

// Update replaces the geo object with the given ID. A nil ExternalID keeps
// the stored one. It returns ErrNotFound if it does not exist, and the same
// reference and conflict errors as Create.
func (st *GeoObjectStore) Update(ctx context.Context, id uuid.UUID, g GeoObject) (GeoObject, error) {
	g.ID = id

	err := st.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkHouseNumberOnStreet(tx, g); err != nil {
			return err
		}

		omitted := []string{"id", "collector_id"}
		if g.ExternalID == nil {
			omitted = append(omitted, "external_id")
		}

		res := tx.Model(&g).Where(manualRow).Select("*").Omit(omitted...).Updates(&g)
		if res.Error != nil {
			return geoObjectWriteError(res.Error)
		}

		if res.RowsAffected == 0 {
			return refusal(tx, "geo object", nil, "geo_objects", "id = ?", id)
		}

		return tx.Take(&g, "id = ?", id).Error
	})
	if err != nil {
		return GeoObject{}, err
	}

	return g, nil
}

// GetByExternalID returns the geo object with the given external ID, or
// ErrNotFound.
func (st *GeoObjectStore) GetByExternalID(ctx context.Context, externalID string) (GeoObject, error) {
	var g GeoObject

	err := st.db.WithContext(ctx).Take(&g, "external_id = ?", externalID).Error

	return g, notFound(err)
}

// UpsertByExternalID creates the geo object with the given external ID, or
// replaces it if it exists, keeping its ID. It reports whether the object was
// created and returns the same reference errors as Create, and ErrNameTaken
// if another geo object already has g's name in its group.
func (st *GeoObjectStore) UpsertByExternalID(
	ctx context.Context,
	externalID string,
	g GeoObject,
) (GeoObject, bool, error) {
	g.ExternalID = &externalID
	g.CollectorID = nil

	var created bool

	err := st.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := checkHouseNumberOnStreet(tx, g); err != nil {
			return err
		}

		var existing GeoObject

		err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Select("id", "collector_id").
			Take(&existing, "external_id = ?", externalID).Error

		switch {
		case err == nil && existing.CollectorID != nil:
			return ownedBy([]string{*existing.CollectorID}, "the geo object belongs to %s; only its runs can change it")
		case errors.Is(err, gorm.ErrRecordNotFound):
			created = true
			g.ID = uuid.Nil
			err = tx.Create(&g).Error
		case err == nil:
			g.ID = existing.ID
			err = tx.Model(&g).Clauses(clause.Returning{}).Select("*").Omit("id", "collector_id").Updates(&g).Error
		}

		return geoObjectWriteError(err)
	})
	if err != nil {
		return GeoObject{}, false, err
	}

	return g, created, nil
}

// DeleteByExternalID removes the geo object with the given external ID. It
// returns ErrNotFound if there is none, and an error matching
// ErrOwnedByCollector if a collector owns it.
func (st *GeoObjectStore) DeleteByExternalID(ctx context.Context, externalID string) error {
	return st.deleteOne(ctx, "external_id = ?", externalID)
}

// Delete removes the geo object with the given ID. It returns ErrNotFound if
// there is none, and an error matching ErrOwnedByCollector if a collector
// owns it.
func (st *GeoObjectStore) Delete(ctx context.Context, id uuid.UUID) error {
	return st.deleteOne(ctx, "id = ?", id)
}

func (st *GeoObjectStore) deleteOne(ctx context.Context, cond string, arg any) error {
	db := st.db.WithContext(ctx)

	err := requireOneRow(db.Where(manualRow).Delete(&GeoObject{}, cond, arg))
	if errors.Is(err, ErrNotFound) {
		return refusal(db, "geo object", nil, "geo_objects", cond, arg)
	}

	return err
}

// MaxBulkDeleteGeoObjects caps the IDs plus external IDs of one
// GeoObjectRefs, keeping the DELETE well below PostgreSQL's limit of 65535
// bind parameters.
const MaxBulkDeleteGeoObjects = 10000

// GeoObjectRefs names geo objects by ID, by external ID, or both.
type GeoObjectRefs struct {
	IDs         []uuid.UUID `json:"ids"`
	ExternalIDs []string    `json:"externalIds"`
}

// Validate reports whether r names between 1 and MaxBulkDeleteGeoObjects geo
// objects with valid IDs and external IDs.
func (r GeoObjectRefs) Validate() error {
	total := len(r.IDs) + len(r.ExternalIDs)
	if total == 0 {
		return errors.New("at least one id or externalId is required")
	}

	if total > MaxBulkDeleteGeoObjects {
		return fmt.Errorf("at most %d ids and externalIds are allowed", MaxBulkDeleteGeoObjects)
	}

	for i, id := range r.IDs {
		if id == uuid.Nil {
			return fmt.Errorf("ids[%d] must be a valid id", i)
		}
	}

	for i, externalID := range r.ExternalIDs {
		if err := (GeoObject{ExternalID: &externalID}).ValidateExternalID(); err != nil {
			return fmt.Errorf("externalIds[%d]: %w", i, err)
		}
	}

	return nil
}

// DeleteMany removes every geo object named by refs and returns how many it
// removed. Refs that match no geo object are skipped, so a repeated call is
// harmless. If a collector owns any of them, none is removed and the error
// matches ErrOwnedByCollector.
func (st *GeoObjectStore) DeleteMany(ctx context.Context, refs GeoObjectRefs) (int64, error) {
	var deleted int64

	err := st.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owners []string

		err := refs.where(tx.Model(&GeoObject{})).Where("collector_id IS NOT NULL").
			Distinct("collector_id").Order("collector_id").Pluck("collector_id", &owners).Error
		if err != nil {
			return err
		}

		if len(owners) > 0 {
			return ownedBy(owners, "some of the geo objects belong to %s; only its runs can change them")
		}

		res := refs.where(tx).Delete(&GeoObject{})
		deleted = res.RowsAffected

		return res.Error
	})

	return deleted, err
}

// where narrows query to the geo objects r names.
func (r GeoObjectRefs) where(query *gorm.DB) *gorm.DB {
	switch {
	case len(r.IDs) > 0 && len(r.ExternalIDs) > 0:
		return query.Where("id IN ? OR external_id IN ?", r.IDs, r.ExternalIDs)
	case len(r.IDs) > 0:
		return query.Where("id IN ?", r.IDs)
	default:
		return query.Where("external_id IN ?", r.ExternalIDs)
	}
}

// checkHouseNumberOnStreet verifies that g's house number, if any, exists and
// lies on g's street. It must run inside tx: the row is locked FOR SHARE until
// tx ends, so the house number cannot move to another street before g is
// written.
func checkHouseNumberOnStreet(tx *gorm.DB, g GeoObject) error {
	if g.HouseNumberID == nil {
		return nil
	}

	var h HouseNumber

	err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthShare}).
		Select("street_id").
		Take(&h, "id = ?", *g.HouseNumberID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrHouseNumberNotFound
	}

	if err != nil {
		return err
	}

	if h.StreetID != g.StreetID {
		return ErrHouseNumberNotOnStreet
	}

	return nil
}
