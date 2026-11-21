package store

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

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
	Settings     map[string]json.RawMessage `json:"-" gorm:"type:jsonb;serializer:json;not null;default:'{}'"`
	DiscoveredAt time.Time                  `json:"discoveredAt" gorm:"not null"`
	UpdatedAt    time.Time                  `json:"updatedAt" gorm:"not null"`

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

// collectorRunsActiveKey is the unique index that allows one running run per
// collector, created by Migrate.
const collectorRunsActiveKey = "collector_runs_active_key"

// MaxRunIssues caps the issues of each kind stored with a run.
const MaxRunIssues = 1000
