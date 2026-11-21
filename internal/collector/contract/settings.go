package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// SettingType is the type of a collector setting's value.
type SettingType string

// The setting types of contract version 1.
const (
	SettingString  SettingType = "string"
	SettingSecret  SettingType = "secret"
	SettingInteger SettingType = "integer"
	SettingNumber  SettingType = "number"
	SettingBoolean SettingType = "boolean"
)

// SettingSpec declares one setting a collector accepts.
type SettingSpec struct {
	Key         string          `json:"key"`
	Label       LocalizedText   `json:"label"`
	Description LocalizedText   `json:"description,omitempty"`
	Type        SettingType     `json:"type"`
	Required    bool            `json:"required,omitempty"`
	Default     json.RawMessage `json:"default,omitempty"`
}

// Validate reports the first rule of the contract that s breaks.
func (s SettingSpec) Validate() error {
	if !settingKeyPattern.MatchString(s.Key) {
		return fmt.Errorf("key %q must match %s", s.Key, settingKeyPattern)
	}

	if err := s.Label.validate(maxLabelLength); err != nil {
		return fmt.Errorf("label: %w", err)
	}

	if s.Description != nil {
		if err := s.Description.validate(maxDescriptionLength); err != nil {
			return fmt.Errorf("description: %w", err)
		}
	}

	switch s.Type {
	case SettingString, SettingInteger, SettingNumber, SettingBoolean:
	case SettingSecret:
		if s.Default != nil {
			return errors.New("a secret setting can't have a default")
		}
	default:
		return fmt.Errorf("unknown type %q", s.Type)
	}

	if s.Default != nil {
		if err := s.ValidateValue(s.Default); err != nil {
			return fmt.Errorf("default: %w", err)
		}
	}

	return nil
}

// ValidateValue reports whether v, a JSON value, fits s's type. Strings must
// be non-empty: an unset setting is omitted rather than set to "".
func (s SettingSpec) ValidateValue(v json.RawMessage) error {
	_, err := s.EnvValue(v)
	return err
}

// EnvValue renders v as the value of the setting's COLLECTOR_SETTING_<KEY>
// environment variable: strings as they are, numbers in decimal notation and
// booleans as "true" or "false".
func (s SettingSpec) EnvValue(v json.RawMessage) (string, error) {
	switch s.Type {
	case SettingString, SettingSecret:
		var str string
		if err := json.Unmarshal(v, &str); err != nil || str == "" {
			return "", errors.New("must be a non-empty string")
		}

		return str, nil
	case SettingInteger:
		n, err := decodeNumber(v)
		if err != nil {
			return "", errors.New("must be an integer")
		}

		i, err := strconv.ParseInt(n.String(), 10, 64)
		if err != nil {
			return "", errors.New("must be an integer")
		}

		return strconv.FormatInt(i, 10), nil
	case SettingNumber:
		n, err := decodeNumber(v)
		if err != nil {
			return "", errors.New("must be a number")
		}

		f, err := n.Float64()
		if err != nil || math.IsInf(f, 0) {
			return "", errors.New("must be a finite number")
		}

		return strconv.FormatFloat(f, 'f', -1, 64), nil
	case SettingBoolean:
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			return "", errors.New("must be true or false")
		}

		return strconv.FormatBool(b), nil
	default:
		return "", fmt.Errorf("unknown type %q", s.Type)
	}
}

// EnvName returns the name of the environment variable that carries the
// setting's value to the collector.
func (s SettingSpec) EnvName() string {
	return "COLLECTOR_SETTING_" + strings.ToUpper(s.Key)
}

// decodeNumber parses v as a JSON number, keeping its literal text so that
// integers beyond float64 precision survive.
func decodeNumber(v json.RawMessage) (json.Number, error) {
	dec := json.NewDecoder(bytes.NewReader(v))
	dec.UseNumber()

	var n any
	if err := dec.Decode(&n); err != nil {
		return "", err
	}

	num, ok := n.(json.Number)
	if !ok {
		return "", errors.New("not a number")
	}

	return num, nil
}
