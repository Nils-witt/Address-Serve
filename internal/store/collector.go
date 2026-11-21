package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

// CollectorStatus says whether a collector can be run.
type CollectorStatus string

// The states of a discovered collector.
const (
	// CollectorOK is a collector whose executable was found and described
	// itself validly.
	CollectorOK CollectorStatus = "ok"
	// CollectorInvalid is a collector whose executable was found but whose
	// ID or describe output was rejected; see Collector.Error.
	CollectorInvalid CollectorStatus = "invalid"
	// CollectorMissing is a collector whose executable is gone. Its settings
	// and owned rows are kept until it is deleted.
	CollectorMissing CollectorStatus = "missing"
)

// Collector is an executable in the collectors folder that returns addresses
// and geo objects to sync. Its ID is derived from its filename.
type Collector struct {
	ID          string                 `json:"id" gorm:"primaryKey"`
	Path        string                 `json:"path" gorm:"not null"`
	Status      CollectorStatus        `json:"status" gorm:"not null"`
	Error       *string                `json:"error,omitempty"`
	Name        string                 `json:"name" gorm:"not null;default:''"`
	Version     string                 `json:"version" gorm:"not null;default:''"`
	Description contract.LocalizedText `json:"description,omitempty" gorm:"type:jsonb;serializer:json"`
	// SettingsSchema is the settings list of the collector's last valid
	// describe output.
	SettingsSchema []contract.SettingSpec `json:"settingsSchema" gorm:"type:jsonb;serializer:json;not null;default:'[]'"`
	// Settings holds the configured values by setting key, secrets included,
	// so it is never serialized as is; the API renders it with secrets masked.
	Settings map[string]json.RawMessage `json:"-" gorm:"type:jsonb;serializer:json;not null;default:'{}'"`
	// Schedule is the cron expression that starts runs, empty for none; see
	// ParseSchedule.
	Schedule string `json:"schedule" gorm:"not null;default:''"`
	// ScheduledUntil is when the schedule was last set or last started a run:
	// the next run is due at the schedule's first time after it.
	ScheduledUntil *time.Time `json:"-"`
	DiscoveredAt   time.Time  `json:"discoveredAt" gorm:"not null"`
	UpdatedAt      time.Time  `json:"updatedAt" gorm:"not null"`

	// The fields below are never loaded; they declare the foreign keys from
	// the rows a collector owns. Deleting a collector turns its rows into
	// manual ones and removes its runs.
	Streets      []Street       `json:"-" gorm:"foreignKey:CollectorID;constraint:OnDelete:SET NULL"`
	HouseNumbers []HouseNumber  `json:"-" gorm:"foreignKey:CollectorID;constraint:OnDelete:SET NULL"`
	GeoObjects   []GeoObject    `json:"-" gorm:"foreignKey:CollectorID;constraint:OnDelete:SET NULL"`
	Runs         []CollectorRun `json:"-" gorm:"constraint:OnDelete:CASCADE"`
}

// RunStatus is the state of a collector run.
type RunStatus string

// The states of a collector run. Only RunRunning is not final.
const (
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

// RunPhase is the part of a running collector run that is in progress.
type RunPhase string

// The phases of a collector run.
const (
	// PhaseCollecting is while the collector process runs.
	PhaseCollecting RunPhase = "collecting"
	// PhaseApplying is while its output is synced into the database.
	PhaseApplying RunPhase = "applying"
)

// CollectorRun is one execution of a collector and its outcome.
type CollectorRun struct {
	ID          uuid.UUID  `json:"id" gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	CollectorID string     `json:"collectorId" gorm:"not null"`
	Status      RunStatus  `json:"status" gorm:"not null"`
	Phase       *RunPhase  `json:"phase,omitempty"`
	TriggeredBy string     `json:"triggeredBy" gorm:"not null"`
	StartedAt   time.Time  `json:"startedAt" gorm:"not null"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
	ExitCode    *int       `json:"exitCode,omitempty"`
	Error       *string    `json:"error,omitempty"`
	Stats       *SyncStats `json:"stats,omitempty" gorm:"type:jsonb;serializer:json"`
	// Issues lists the first MaxRunIssues entries of each kind; Stats holds
	// the full counts.
	Issues []SyncIssue `json:"issues,omitempty" gorm:"type:jsonb;serializer:json"`
	// Log is the tail of the collector's stderr.
	Log string `json:"log,omitempty" gorm:"not null;default:''"`
}

// EntityStats counts what a sync did to one kind of row.
type EntityStats struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Adopted   int `json:"adopted"`
	Deleted   int `json:"deleted"`
	Released  int `json:"released"`
	Skipped   int `json:"skipped"`
	Conflicts int `json:"conflicts"`
}

// SyncStats counts what a sync did, per kind of row.
type SyncStats struct {
	Streets      EntityStats `json:"streets"`
	HouseNumbers EntityStats `json:"houseNumbers"`
	GeoObjects   EntityStats `json:"geoObjects"`
}

// IssueKind classifies a SyncIssue.
type IssueKind string

// The kinds of SyncIssue.
const (
	// IssueSkipped is an output entry that was not applied: it was invalid,
	// a duplicate, or its geo object could not be placed.
	IssueSkipped IssueKind = "skipped"
	// IssueConflict is an output entry that matches a row owned by someone
	// else, which was left untouched.
	IssueConflict IssueKind = "conflict"
	// IssueAdopted is a manual house number the collector took over.
	IssueAdopted IssueKind = "adopted"
	// IssueReleased is an owned row the collector no longer returns but that
	// could not be deleted because foreign rows depend on it; it became a
	// manual row.
	IssueReleased IssueKind = "released"
)

// IssueEntity names what a SyncIssue is about.
type IssueEntity string

// The entities a SyncIssue can be about.
const (
	EntityAddress     IssueEntity = "address"
	EntityGeoObject   IssueEntity = "geoObject"
	EntityStreet      IssueEntity = "street"
	EntityHouseNumber IssueEntity = "houseNumber"
)

// SyncIssue is one noteworthy thing that happened during a sync.
type SyncIssue struct {
	Kind   IssueKind   `json:"kind"`
	Entity IssueEntity `json:"entity"`
	// Index is the entry's position in the collector output's addresses or
	// geoObjects, when the issue is about an output entry.
	Index *int `json:"index,omitempty"`
	// Key describes the entry or row in human-readable form.
	Key    string     `json:"key"`
	Reason string     `json:"reason"`
	RowID  *uuid.UUID `json:"rowId,omitempty"`
}

// Column names shared by collectors and collector_runs.
const (
	columnStatus = "status"
	columnError  = "error"
)

// collectorRunsActiveKey is the unique index that allows one running run per
// collector, created by Migrate.
const collectorRunsActiveKey = "collector_runs_active_key"

// MaxRunIssues caps the issues of each kind stored with a run.
const MaxRunIssues = 1000

var (
	// ErrCollectorAvailable is returned when deleting a collector whose
	// executable is still present and valid.
	ErrCollectorAvailable = errors.New("only missing or invalid collectors can be deleted")

	// ErrRunActive is returned when a collector already has a running run.
	ErrRunActive = errors.New("the collector already has a running run")

	// ErrInvalidSettings is wrapped by the errors of settings that don't fit
	// the collector's schema.
	ErrInvalidSettings = errors.New("invalid settings")
)

// CollectorStore reads and writes collectors, their runs, and syncs their
// output.
type CollectorStore struct {
	db *gorm.DB
}

// NewCollectorStore returns a CollectorStore backed by db.
func NewCollectorStore(db *gorm.DB) *CollectorStore {
	return &CollectorStore{db: db}
}

// Discovered is a collector found in the collectors folder. Description is
// nil when the collector is invalid, in which case Error says why.
type Discovered struct {
	ID          string
	Path        string
	Description *contract.Description
	Error       string
}

// SaveDiscovered records the outcome of scanning the collectors folder: found
// collectors are created or updated, and stored ones that weren't found are
// marked missing. Settings are kept. An invalid collector keeps the metadata
// of its last valid description, so the UI can still name it.
func (cs *CollectorStore) SaveDiscovered(ctx context.Context, found []Discovered) error {
	now := time.Now()

	return cs.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := make([]string, len(found))

		for i, d := range found {
			ids[i] = d.ID
			c := Collector{ID: d.ID, Path: d.Path, Status: CollectorOK, DiscoveredAt: now, UpdatedAt: now}
			columns := []string{"path", columnStatus, columnError, "updated_at"}

			if d.Description == nil {
				c.Status, c.Error = CollectorInvalid, &d.Error
			} else {
				c.Name, c.Version = d.Description.Name, d.Description.Version
				c.Description = d.Description.Description
				c.SettingsSchema = d.Description.Settings

				columns = append(columns, "name", "version", "description", "settings_schema")
			}

			if c.SettingsSchema == nil {
				c.SettingsSchema = []contract.SettingSpec{}
			}

			c.Settings = map[string]json.RawMessage{}

			err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoUpdates: clause.AssignmentColumns(columns),
			}).Create(&c).Error
			if err != nil {
				return fmt.Errorf("save collector %s: %w", d.ID, err)
			}
		}

		missing := tx.Model(&Collector{}).Where("status <> ?", CollectorMissing)
		if len(ids) > 0 {
			missing = missing.Where("id NOT IN ?", ids)
		}

		return missing.Updates(map[string]any{columnStatus: CollectorMissing, columnError: nil, "updated_at": now}).Error
	})
}

// List returns every collector, ordered by ID.
func (cs *CollectorStore) List(ctx context.Context) ([]Collector, error) {
	collectors := []Collector{}
	err := cs.db.WithContext(ctx).Order("id").Find(&collectors).Error

	return collectors, err
}

// Get returns the collector with the given ID, or ErrNotFound.
func (cs *CollectorStore) Get(ctx context.Context, id string) (Collector, error) {
	var c Collector

	err := cs.db.WithContext(ctx).Take(&c, "id = ?", id).Error

	return c, notFound(err)
}

// SettingsInput replaces a collector's settings. Absent keys are unset,
// except secrets, which are kept so that clients never need to know their
// value; a JSON null unsets a secret too.
type SettingsInput map[string]json.RawMessage

// UpdateSettings validates input against the collector's schema, applies it,
// and returns the updated collector. It returns ErrNotFound for an unknown
// collector and an error wrapping ErrInvalidSettings for input that doesn't
// fit the schema.
func (cs *CollectorStore) UpdateSettings(ctx context.Context, id string, input SettingsInput) (Collector, error) {
	var c Collector

	err := cs.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Take(&c, "id = ?", id).Error
		if err != nil {
			return notFound(err)
		}

		settings, err := MergeSettings(c.SettingsSchema, c.Settings, input)
		if err != nil {
			return err
		}

		c.Settings, c.UpdatedAt = settings, time.Now()

		return tx.Model(&c).Select("settings", "updated_at").Updates(&c).Error
	})
	if err != nil {
		return Collector{}, err
	}

	return c, nil
}

// MergeSettings returns the settings that result from applying input to
// current under schema; see SettingsInput.
func MergeSettings(
	schema []contract.SettingSpec, current map[string]json.RawMessage, input SettingsInput,
) (map[string]json.RawMessage, error) {
	specs := make(map[string]contract.SettingSpec, len(schema))
	for _, s := range schema {
		specs[s.Key] = s
	}

	for _, key := range slices.Sorted(maps.Keys(input)) {
		if _, ok := specs[key]; !ok {
			return nil, fmt.Errorf("%w: unknown setting %q", ErrInvalidSettings, key)
		}
	}

	merged := make(map[string]json.RawMessage, len(schema))

	for _, spec := range schema {
		value, given := input[spec.Key]

		switch {
		case given && isJSONNull(value):
			continue
		case given:
			if err := spec.ValidateValue(value); err != nil {
				return nil, fmt.Errorf("%w: %s %w", ErrInvalidSettings, spec.Key, err)
			}

			merged[spec.Key] = value
		case spec.Type == contract.SettingSecret && current[spec.Key] != nil:
			merged[spec.Key] = current[spec.Key]
		}
	}

	return merged, nil
}

// MissingRequiredSettings returns the keys of required settings that have
// neither a value nor a default.
func (c Collector) MissingRequiredSettings() []string {
	var missing []string

	for _, s := range c.SettingsSchema {
		if s.Required && c.Settings[s.Key] == nil && s.Default == nil {
			missing = append(missing, s.Key)
		}
	}

	return missing
}

func isJSONNull(v json.RawMessage) bool {
	return strings.TrimSpace(string(v)) == "null"
}

// Delete removes a missing or invalid collector and its runs. Its rows
// become manual ones, unless deleteData is set: then they are deleted as if
// the collector had returned nothing, so rows that others depend on are
// released rather than deleted. It returns ErrNotFound, ErrCollectorAvailable
// for a collector that can still run, and ErrRunActive while it runs.
func (cs *CollectorStore) Delete(ctx context.Context, id string, deleteData bool) error {
	return cs.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c Collector

		err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).Take(&c, "id = ?", id).Error
		if err != nil {
			return notFound(err)
		}

		if c.Status == CollectorOK {
			return ErrCollectorAvailable
		}

		var running int64
		if err := tx.Model(&CollectorRun{}).Where("collector_id = ? AND status = ?", id, RunRunning).Count(&running).Error; err != nil {
			return err
		}

		if running > 0 {
			return ErrRunActive
		}

		if deleteData {
			if _, _, err := syncTx(tx, id, contract.Output{}, SyncOptions{}); err != nil {
				return fmt.Errorf("delete data of collector %s: %w", id, err)
			}
		}

		return tx.Delete(&Collector{}, "id = ?", id).Error
	})
}
