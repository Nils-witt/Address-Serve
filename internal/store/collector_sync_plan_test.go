package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

func planAddr(street string, number int, lat, lon float64) contract.Address {
	return contract.Address{
		Street: street, District: "D", City: "C", Country: "X",
		Number: number, Postcode: "12345", Latitude: new(lat), Longitude: new(lon),
	}
}

func TestPlanAddresses(t *testing.T) {
	t.Parallel()

	withSID := func(a contract.Address, id string) contract.Address { a.StreetExternalID = &id; return a }
	withHID := func(a contract.Address, id string) contract.Address { a.HouseNumberExternalID = &id; return a }
	invalid := planAddr("A", 1, 1, 1)
	invalid.Postcode = ""

	tests := []struct {
		name        string
		in          []contract.Address
		wantStreets []string // planned streets with their address counts, "skip" marks skipped ones
		wantSkipped int
		wantReason  string
	}{
		{
			name:        "groups by street in order of appearance",
			in:          []contract.Address{planAddr("B", 1, 1, 1), planAddr("A", 1, 1, 1), planAddr("B", 2, 1, 1)},
			wantStreets: []string{"B:2", "A:1"},
		},
		{
			name:        "invalid entry is skipped",
			in:          []contract.Address{invalid, planAddr("A", 2, 1, 1)},
			wantStreets: []string{"A:1"},
			wantSkipped: 1,
			wantReason:  "postcode",
		},
		{
			name:        "repeated number: first wins",
			in:          []contract.Address{planAddr("R", 1, 1, 1), planAddr("R", 1, 2, 2)},
			wantStreets: []string{"R:1"},
			wantSkipped: 1,
			wantReason:  "duplicate",
		},
		{
			name:        "repeated house number id: first wins",
			in:          []contract.Address{withHID(planAddr("A", 1, 1, 1), "h"), withHID(planAddr("B", 2, 1, 1), "h")},
			wantStreets: []string{"A:1"},
			wantSkipped: 1,
			wantReason:  "duplicate",
		},
		{
			name:        "street ids disagree within a street",
			in:          []contract.Address{withSID(planAddr("A", 1, 1, 1), "s1"), withSID(planAddr("A", 2, 1, 1), "s2"), planAddr("B", 1, 1, 1)},
			wantStreets: []string{"A:skip", "B:1"},
			wantSkipped: 2,
			wantReason:  "inconsistent street",
		},
		{
			name:        "street id given on some addresses only",
			in:          []contract.Address{withSID(planAddr("A", 1, 1, 1), "s1"), planAddr("A", 2, 1, 1)},
			wantStreets: []string{"A:skip"},
			wantSkipped: 2,
			wantReason:  "inconsistent street",
		},
		{
			name:        "one street id for two streets",
			in:          []contract.Address{withSID(planAddr("A", 1, 1, 1), "s1"), withSID(planAddr("B", 1, 1, 1), "s1")},
			wantStreets: []string{"A:skip", "B:skip"},
			wantSkipped: 2,
			wantReason:  "inconsistent street",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := &syncer{issues: newIssueLog()}
			groups := s.planAddresses(tt.in)

			var got []string

			for _, g := range groups {
				desc := fmt.Sprintf("%s:%d", g.key.Name, len(g.addresses))
				if g.skip {
					desc = g.key.Name + ":skip"
				}

				got = append(got, desc)
			}

			if strings.Join(got, ",") != strings.Join(tt.wantStreets, ",") {
				t.Errorf("streets = %v, want %v", got, tt.wantStreets)
			}

			if s.stats.HouseNumbers.Skipped != tt.wantSkipped || len(s.issues.list) != tt.wantSkipped {
				t.Errorf("skipped = %d with %d issues, want %d", s.stats.HouseNumbers.Skipped, len(s.issues.list), tt.wantSkipped)
			}

			for _, issue := range s.issues.list {
				if !strings.Contains(issue.Reason, tt.wantReason) || issue.Index == nil {
					t.Errorf("issue = %+v, want an indexed issue mentioning %q", issue, tt.wantReason)
				}
			}
		})
	}
}

func TestPlanGeoObjects(t *testing.T) {
	t.Parallel()

	geo := func(group, name string, ext *string) contract.GeoObject {
		return contract.GeoObject{Group: group, Name: name, ExternalID: ext, Latitude: new(1.0), Longitude: new(1.0)}
	}

	s := &syncer{issues: newIssueLog()}
	planned := s.planGeoObjects([]contract.GeoObject{
		geo("g", "a", nil),
		geo("g", "a", nil),            // same name
		geo("g", "b", new("x")),       // ok
		geo("h", "c", new("x")),       // same external ID
		{Group: "g", Name: "invalid"}, // no coordinates
		geo("h", "a", nil),            // same name, other group: ok
	})

	if len(planned) != 3 || planned[0].index != 0 || planned[1].index != 2 || planned[2].index != 5 {
		t.Errorf("planned = %+v, want entries 0, 2 and 5", planned)
	}

	if s.stats.GeoObjects.Skipped != 3 {
		t.Errorf("skipped = %d, want 3", s.stats.GeoObjects.Skipped)
	}
}

func TestMedoid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		points           [][2]float64
		wantLat, wantLon float64
	}{
		{name: "single point", points: [][2]float64{{50, 7}}, wantLat: 50, wantLon: 7},
		{name: "middle of a line", points: [][2]float64{{50, 7}, {50.002, 7}, {50.001, 7}}, wantLat: 50.001, wantLon: 7},
		{
			name: "closest to the centroid, not the centroid",
			// The centroid of an L lies off both legs; the corner is closest.
			points:  [][2]float64{{50, 7}, {50.003, 7}, {50, 7.003}, {50.0005, 7.0005}},
			wantLat: 50.0005, wantLon: 7.0005,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			addresses := make([]plannedAddress, len(tt.points))
			for i, p := range tt.points {
				addresses[i] = plannedAddress{index: i, addr: planAddr("A", i+1, p[0], p[1])}
			}

			lat, lon := medoid(addresses)
			if lat != tt.wantLat || lon != tt.wantLon {
				t.Errorf("medoid = %v, %v, want %v, %v", lat, lon, tt.wantLat, tt.wantLon)
			}
		})
	}
}

func TestIssueLog_CapsEachKind(t *testing.T) {
	t.Parallel()

	l := newIssueLog()
	for range MaxRunIssues + 5 {
		l.add(SyncIssue{Kind: IssueSkipped})
	}

	l.add(SyncIssue{Kind: IssueConflict})

	if len(l.list) != MaxRunIssues+1 || l.counts[IssueSkipped] != MaxRunIssues+5 {
		t.Errorf("kept %d issues and counted %d skipped, want %d and %d",
			len(l.list), l.counts[IssueSkipped], MaxRunIssues+1, MaxRunIssues+5)
	}
}

func TestMergeSettings(t *testing.T) {
	t.Parallel()

	schema := []contract.SettingSpec{
		{Key: "url", Type: contract.SettingString},
		{Key: "key", Type: contract.SettingSecret},
		{Key: "limit", Type: contract.SettingInteger},
	}
	current := map[string]json.RawMessage{"url": json.RawMessage(`"old"`), "key": json.RawMessage(`"s3cret"`)}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "replace, keeping the secret", input: `{"url":"new"}`, want: `{"key":"s3cret","url":"new"}`},
		{name: "absent non-secret is unset", input: `{}`, want: `{"key":"s3cret"}`},
		{name: "replace the secret", input: `{"key":"other"}`, want: `{"key":"other"}`},
		{name: "null clears the secret", input: `{"key":null,"limit":5}`, want: `{"limit":5}`},
		{name: "unknown key", input: `{"nope":1}`, wantErr: true},
		{name: "wrong type", input: `{"limit":"5"}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var input SettingsInput
			if err := json.Unmarshal([]byte(tt.input), &input); err != nil {
				t.Fatal(err)
			}

			got, err := MergeSettings(schema, current, input)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidSettings) {
					t.Errorf("err = %v, want ErrInvalidSettings", err)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			b, _ := json.Marshal(got)
			if string(b) != tt.want {
				t.Errorf("settings = %s, want %s", b, tt.want)
			}
		})
	}
}

func TestCollector_MissingRequiredSettings(t *testing.T) {
	t.Parallel()

	c := Collector{
		SettingsSchema: []contract.SettingSpec{
			{Key: "a", Required: true},
			{Key: "b", Required: true, Default: json.RawMessage(`"d"`)},
			{Key: "c", Required: true},
			{Key: "d"},
		},
		Settings: map[string]json.RawMessage{"c": json.RawMessage(`"x"`)},
	}

	if got := c.MissingRequiredSettings(); len(got) != 1 || got[0] != "a" {
		t.Errorf("MissingRequiredSettings = %v, want [a]", got)
	}
}
