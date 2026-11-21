// Package contract defines version 1 of the collector contract: what a
// collector executable prints for `describe` and `collect`, and how those
// documents are validated. See docs/collectors/contract.md.
//
// It is a leaf package so that both the store, which syncs collect outputs,
// and the runner, which executes collectors, can share these types.
package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// Version is the contract version this app speaks.
const Version = 1

const (
	maxNameLength        = 100
	maxVersionLength     = 50
	maxDescriptionLength = 1000
	maxLabelLength       = 200
	maxExternalIDLength  = 255
)

var (
	settingKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	languagePattern   = regexp.MustCompile(`^[a-z]{2}$`)
)

// errUnsupportedVersion is wrapped by Description.Validate and DecodeOutput
// when a document declares a contract version other than Version.
var errUnsupportedVersion = errors.New("unsupported contractVersion")

// Description is what `<exe> describe` prints.
type Description struct {
	ContractVersion int           `json:"contractVersion"`
	Name            string        `json:"name"`
	Version         string        `json:"version"`
	Description     LocalizedText `json:"description,omitempty"`
	Settings        []SettingSpec `json:"settings"`
}

// Validate reports the first rule of the contract that d breaks.
func (d Description) Validate() error {
	if d.ContractVersion != Version {
		return fmt.Errorf("%w %d, want %d", errUnsupportedVersion, d.ContractVersion, Version)
	}

	if d.Name == "" || len(d.Name) > maxNameLength {
		return fmt.Errorf("name must be 1 to %d characters long", maxNameLength)
	}

	if d.Version == "" || len(d.Version) > maxVersionLength {
		return fmt.Errorf("version must be 1 to %d characters long", maxVersionLength)
	}

	if d.Description != nil {
		if err := d.Description.validate(maxDescriptionLength); err != nil {
			return fmt.Errorf("description: %w", err)
		}
	}

	seen := make(map[string]struct{}, len(d.Settings))

	for i, s := range d.Settings {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("settings[%d]: %w", i, err)
		}

		if _, dup := seen[s.Key]; dup {
			return fmt.Errorf("settings[%d]: duplicate key %q", i, s.Key)
		}

		seen[s.Key] = struct{}{}
	}

	return nil
}

// DecodeDescription parses and validates the stdout of `<exe> describe`.
func DecodeDescription(data []byte) (Description, error) {
	var d Description
	if err := json.Unmarshal(data, &d); err != nil {
		return Description{}, fmt.Errorf("parse describe output: %w", err)
	}

	if err := d.Validate(); err != nil {
		return Description{}, err
	}

	return d, nil
}

// Output is what `<exe> collect` prints.
type Output struct {
	ContractVersion int         `json:"contractVersion"`
	Addresses       []Address   `json:"addresses"`
	GeoObjects      []GeoObject `json:"geoObjects"`
}

// DecodeOutput parses the stdout of `<exe> collect`. Only the document as a
// whole is checked here: a broken document fails the run, while invalid rows
// are skipped individually during the sync (see Address.Validate).
func DecodeOutput(data []byte) (Output, error) {
	var o Output
	if err := json.Unmarshal(data, &o); err != nil {
		return Output{}, fmt.Errorf("parse collect output: %w", err)
	}

	if o.ContractVersion != Version {
		return Output{}, fmt.Errorf("%w %d, want %d", errUnsupportedVersion, o.ContractVersion, Version)
	}

	return o, nil
}

// StreetKey is a street's natural key.
type StreetKey struct {
	Name     string
	District string
	City     string
	Country  string
}

// Address is one house number on a street.
type Address struct {
	StreetExternalID      *string  `json:"streetExternalId,omitempty"`
	HouseNumberExternalID *string  `json:"houseNumberExternalId,omitempty"`
	Street                string   `json:"street"`
	District              string   `json:"district"`
	City                  string   `json:"city"`
	Country               string   `json:"country"`
	Number                int      `json:"number"`
	NumberAddition        *string  `json:"numberAddition,omitempty"`
	Postcode              string   `json:"postcode"`
	Latitude              *float64 `json:"latitude"`
	Longitude             *float64 `json:"longitude"`
}

// StreetKey returns the natural key of a's street.
func (a Address) StreetKey() StreetKey {
	return StreetKey{Name: a.Street, District: a.District, City: a.City, Country: a.Country}
}

// Validate reports the first rule of the contract that a breaks.
func (a Address) Validate() error {
	if a.Street == "" || a.District == "" || a.City == "" || a.Country == "" {
		return errors.New("street, district, city and country are required")
	}

	if a.Number <= 0 {
		return errors.New("number must be positive")
	}

	if a.NumberAddition != nil && *a.NumberAddition == "" {
		return errors.New("numberAddition must be omitted, null or non-empty")
	}

	if a.Postcode == "" {
		return errors.New("postcode is required")
	}

	if err := validateExternalID("streetExternalId", a.StreetExternalID); err != nil {
		return err
	}

	if err := validateExternalID("houseNumberExternalId", a.HouseNumberExternalID); err != nil {
		return err
	}

	return validateCoordinates(a.Latitude, a.Longitude)
}

// AddressRef points a geo object at a street and optionally one of its house
// numbers.
type AddressRef struct {
	StreetExternalID      *string `json:"streetExternalId,omitempty"`
	HouseNumberExternalID *string `json:"houseNumberExternalId,omitempty"`
	Street                string  `json:"street"`
	District              string  `json:"district"`
	City                  string  `json:"city"`
	Country               string  `json:"country"`
	Number                *int    `json:"number,omitempty"`
	NumberAddition        *string `json:"numberAddition,omitempty"`
}

// StreetKey returns the natural key of the referenced street.
func (r AddressRef) StreetKey() StreetKey {
	return StreetKey{Name: r.Street, District: r.District, City: r.City, Country: r.Country}
}

// Validate reports the first rule of the contract that r breaks.
func (r AddressRef) Validate() error {
	if r.Street == "" || r.District == "" || r.City == "" || r.Country == "" {
		return errors.New("address: street, district, city and country are required")
	}

	if r.Number != nil && *r.Number <= 0 {
		return errors.New("address: number must be positive")
	}

	if r.NumberAddition != nil && (*r.NumberAddition == "" || r.Number == nil) {
		return errors.New("address: numberAddition must be non-empty and needs a number")
	}

	if err := validateExternalID("address.streetExternalId", r.StreetExternalID); err != nil {
		return err
	}

	return validateExternalID("address.houseNumberExternalId", r.HouseNumberExternalID)
}

// GeoObject is a named, grouped point.
type GeoObject struct {
	ExternalID *string     `json:"externalId,omitempty"`
	Group      string      `json:"group"`
	Name       string      `json:"name"`
	Latitude   *float64    `json:"latitude"`
	Longitude  *float64    `json:"longitude"`
	Address    *AddressRef `json:"address,omitempty"`
}

// Validate reports the first rule of the contract that g breaks.
func (g GeoObject) Validate() error {
	if g.Group == "" || g.Name == "" {
		return errors.New("group and name are required")
	}

	if err := validateExternalID("externalId", g.ExternalID); err != nil {
		return err
	}

	if err := validateCoordinates(g.Latitude, g.Longitude); err != nil {
		return err
	}

	if g.Address != nil {
		return g.Address.Validate()
	}

	return nil
}

func validateExternalID(field string, id *string) error {
	if id != nil && (*id == "" || len(*id) > maxExternalIDLength) {
		return fmt.Errorf("%s must be omitted or 1 to %d characters long", field, maxExternalIDLength)
	}

	return nil
}

// validateCoordinates requires both coordinates: they are pointers because 0
// is a valid latitude and longitude, so absence can't be told apart otherwise.
func validateCoordinates(lat, lon *float64) error {
	if lat == nil || lon == nil {
		return errors.New("latitude and longitude are required")
	}

	if *lat < -90 || *lat > 90 {
		return errors.New("latitude must be between -90 and 90")
	}

	if *lon < -180 || *lon > 180 {
		return errors.New("longitude must be between -180 and 180")
	}

	return nil
}
