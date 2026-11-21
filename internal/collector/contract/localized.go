package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// untranslated is the LocalizedText key of a text given as a plain string,
// which applies to every language.
const untranslated = ""

// LocalizedText is either a plain string or an object mapping two-letter
// language codes to strings. A plain string is held under the empty key, and
// is written back as a plain string.
type LocalizedText map[string]string

// UnmarshalJSON accepts a string or an object of strings.
func (t *LocalizedText) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*t = LocalizedText{untranslated: s}
		return nil
	}

	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return errors.New("must be a string or an object of strings")
	}

	*t = m

	return nil
}

// MarshalJSON writes a text given as a plain string back as one.
func (t LocalizedText) MarshalJSON() ([]byte, error) {
	if s, ok := t[untranslated]; ok && len(t) == 1 {
		return json.Marshal(s)
	}

	return json.Marshal(map[string]string(t))
}

// validate requires at least one entry, two-letter language codes and
// non-empty values of at most maxLen bytes.
func (t LocalizedText) validate(maxLen int) error {
	if len(t) == 0 {
		return errors.New("is required")
	}

	_, plain := t[untranslated]
	if plain && len(t) > 1 {
		return errors.New("must be a string or an object, not both")
	}

	// Sorted, so the reported problem is the same on every run.
	for _, lang := range slices.Sorted(maps.Keys(t)) {
		if lang != untranslated && !languagePattern.MatchString(lang) {
			return fmt.Errorf("language %q must be a two-letter code", lang)
		}

		if v := t[lang]; v == "" || len(v) > maxLen {
			return fmt.Errorf("must be 1 to %d characters long", maxLen)
		}
	}

	return nil
}
