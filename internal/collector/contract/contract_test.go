package contract

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDecodeDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		wantErr string
	}{
		{
			name: "minimal",
			json: `{"contractVersion":1,"name":"Example","version":"1.0.0"}`,
		},
		{
			name: "full",
			json: `{"contractVersion":1,"name":"Bonn","version":"2","description":{"en":"House numbers","de":"Hausnummern"},
				"settings":[
					{"key":"source_url","label":"Source URL","type":"string","required":true,"default":"https://example.com"},
					{"key":"api_key","label":{"en":"API key"},"type":"secret"},
					{"key":"limit","label":"Limit","type":"integer","default":10},
					{"key":"radius","label":"Radius","type":"number","default":1.5},
					{"key":"dry","label":"Dry","type":"boolean","default":false}]}`,
		},
		{name: "not json", json: `nope`, wantErr: "parse describe output"},
		{name: "wrong version", json: `{"contractVersion":2,"name":"x","version":"1"}`, wantErr: "unsupported contractVersion"},
		{name: "missing version", json: `{"name":"x","version":"1"}`, wantErr: "unsupported contractVersion"},
		{name: "missing name", json: `{"contractVersion":1,"version":"1"}`, wantErr: "name must be"},
		{name: "name too long", json: `{"contractVersion":1,"name":"` + strings.Repeat("x", 101) + `","version":"1"}`, wantErr: "name must be"},
		{name: "missing collector version", json: `{"contractVersion":1,"name":"x"}`, wantErr: "version must be"},
		{name: "empty description", json: `{"contractVersion":1,"name":"x","version":"1","description":""}`, wantErr: "description: must be 1 to"},
		{name: "bad description language", json: `{"contractVersion":1,"name":"x","version":"1","description":{"eng":"x"}}`, wantErr: "two-letter code"},
		{name: "description wrong type", json: `{"contractVersion":1,"name":"x","version":"1","description":5}`, wantErr: "string or an object"},
		{
			name:    "bad setting key",
			json:    `{"contractVersion":1,"name":"x","version":"1","settings":[{"key":"Source-URL","label":"x","type":"string"}]}`,
			wantErr: "settings[0]: key",
		},
		{
			name:    "duplicate setting key",
			json:    `{"contractVersion":1,"name":"x","version":"1","settings":[{"key":"a","label":"x","type":"string"},{"key":"a","label":"y","type":"string"}]}`,
			wantErr: "settings[1]: duplicate key",
		},
		{
			name:    "missing label",
			json:    `{"contractVersion":1,"name":"x","version":"1","settings":[{"key":"a","type":"string"}]}`,
			wantErr: "label: is required",
		},
		{
			name:    "unknown type",
			json:    `{"contractVersion":1,"name":"x","version":"1","settings":[{"key":"a","label":"x","type":"url"}]}`,
			wantErr: `unknown type "url"`,
		},
		{
			name:    "secret with default",
			json:    `{"contractVersion":1,"name":"x","version":"1","settings":[{"key":"a","label":"x","type":"secret","default":"s"}]}`,
			wantErr: "can't have a default",
		},
		{
			name:    "default of wrong type",
			json:    `{"contractVersion":1,"name":"x","version":"1","settings":[{"key":"a","label":"x","type":"integer","default":"10"}]}`,
			wantErr: "default: must be an integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeDescription([]byte(tt.json))
			checkErr(t, err, tt.wantErr)
		})
	}
}

func TestDecodeOutput(t *testing.T) {
	t.Parallel()

	o, err := DecodeOutput([]byte(`{"contractVersion":1,"addresses":[{"street":"A","number":1}],"geoObjects":[{"group":"g","name":"n"}],"extra":true}`))
	if err != nil {
		t.Fatal(err)
	}

	if len(o.Addresses) != 1 || len(o.GeoObjects) != 1 {
		t.Errorf("got %d addresses and %d geo objects, want 1 and 1", len(o.Addresses), len(o.GeoObjects))
	}

	_, err = DecodeOutput([]byte(`{"contractVersion":3}`))
	checkErr(t, err, "unsupported contractVersion")

	_, err = DecodeOutput([]byte(`{"contractVersion":1,"addresses":{}}`))
	checkErr(t, err, "parse collect output")
}

func validAddress() Address {
	return Address{
		Street: "Main", District: "Center", City: "Town", Country: "DE",
		Number: 1, Postcode: "12345", Latitude: new(50.0), Longitude: new(7.0),
	}
}

func TestAddress_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modify  func(*Address)
		wantErr string
	}{
		{name: "valid", modify: func(*Address) {}},
		{name: "zero coordinates are valid", modify: func(a *Address) { a.Latitude, a.Longitude = new(0.0), new(0.0) }},
		{name: "with ids and addition", modify: func(a *Address) {
			a.StreetExternalID, a.HouseNumberExternalID, a.NumberAddition = new("s1"), new("h1"), new("a")
		}},
		{name: "missing street", modify: func(a *Address) { a.Street = "" }, wantErr: "street, district, city and country"},
		{name: "missing country", modify: func(a *Address) { a.Country = "" }, wantErr: "street, district, city and country"},
		{name: "zero number", modify: func(a *Address) { a.Number = 0 }, wantErr: "number must be positive"},
		{name: "empty addition", modify: func(a *Address) { a.NumberAddition = new("") }, wantErr: "numberAddition"},
		{name: "missing postcode", modify: func(a *Address) { a.Postcode = "" }, wantErr: "postcode"},
		{name: "empty street id", modify: func(a *Address) { a.StreetExternalID = new("") }, wantErr: "streetExternalId"},
		{name: "long house number id", modify: func(a *Address) { a.HouseNumberExternalID = new(strings.Repeat("x", 256)) }, wantErr: "houseNumberExternalId"},
		{name: "missing latitude", modify: func(a *Address) { a.Latitude = nil }, wantErr: "latitude and longitude are required"},
		{name: "latitude out of range", modify: func(a *Address) { a.Latitude = new(91.0) }, wantErr: "latitude must be"},
		{name: "longitude out of range", modify: func(a *Address) { a.Longitude = new(-181.0) }, wantErr: "longitude must be"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := validAddress()
			tt.modify(&a)
			checkErr(t, a.Validate(), tt.wantErr)
		})
	}
}

func TestGeoObject_Validate(t *testing.T) {
	t.Parallel()

	valid := func() GeoObject {
		return GeoObject{Group: "hydrant", Name: "H1", Latitude: new(50.0), Longitude: new(7.0)}
	}
	ref := func() *AddressRef {
		return &AddressRef{Street: "Main", District: "Center", City: "Town", Country: "DE"}
	}

	tests := []struct {
		name    string
		modify  func(*GeoObject)
		wantErr string
	}{
		{name: "valid", modify: func(*GeoObject) {}},
		{name: "with street reference", modify: func(g *GeoObject) { g.Address = ref() }},
		{name: "with full reference", modify: func(g *GeoObject) {
			g.Address = ref()
			g.Address.Number, g.Address.NumberAddition = new(3), new("b")
			g.Address.StreetExternalID, g.Address.HouseNumberExternalID = new("s"), new("h")
		}},
		{name: "missing group", modify: func(g *GeoObject) { g.Group = "" }, wantErr: "group and name"},
		{name: "empty external id", modify: func(g *GeoObject) { g.ExternalID = new("") }, wantErr: "externalId"},
		{name: "missing longitude", modify: func(g *GeoObject) { g.Longitude = nil }, wantErr: "latitude and longitude are required"},
		{name: "incomplete reference", modify: func(g *GeoObject) { g.Address = ref(); g.Address.City = "" }, wantErr: "address: street"},
		{name: "reference number zero", modify: func(g *GeoObject) { g.Address = ref(); g.Address.Number = new(0) }, wantErr: "address: number"},
		{name: "addition without number", modify: func(g *GeoObject) { g.Address = ref(); g.Address.NumberAddition = new("a") }, wantErr: "needs a number"},
		{name: "empty reference id", modify: func(g *GeoObject) { g.Address = ref(); g.Address.StreetExternalID = new("") }, wantErr: "address.streetExternalId"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := valid()
			tt.modify(&g)
			checkErr(t, g.Validate(), tt.wantErr)
		})
	}
}

func TestSettingSpec_EnvValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		typ     SettingType
		value   string
		want    string
		wantErr string
	}{
		{name: "string", typ: SettingString, value: `"hello"`, want: "hello"},
		{name: "secret", typ: SettingSecret, value: `"s3cr3t"`, want: "s3cr3t"},
		{name: "empty string", typ: SettingString, value: `""`, wantErr: "non-empty string"},
		{name: "string given a number", typ: SettingString, value: `5`, wantErr: "non-empty string"},
		{name: "integer", typ: SettingInteger, value: `42`, want: "42"},
		{name: "large integer keeps precision", typ: SettingInteger, value: `9007199254740993`, want: "9007199254740993"},
		{name: "integer given a fraction", typ: SettingInteger, value: `1.5`, wantErr: "must be an integer"},
		{name: "integer given a string", typ: SettingInteger, value: `"1"`, wantErr: "must be an integer"},
		{name: "number", typ: SettingNumber, value: `1.25`, want: "1.25"},
		{name: "number in exponent notation", typ: SettingNumber, value: `1e3`, want: "1000"},
		{name: "number overflows", typ: SettingNumber, value: `1e400`, wantErr: "finite number"},
		{name: "boolean", typ: SettingBoolean, value: `true`, want: "true"},
		{name: "boolean given a string", typ: SettingBoolean, value: `"true"`, wantErr: "true or false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := SettingSpec{Key: "k", Type: tt.typ}.EnvValue(json.RawMessage(tt.value))
			checkErr(t, err, tt.wantErr)

			if got != tt.want {
				t.Errorf("EnvValue = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSettingSpec_EnvName(t *testing.T) {
	t.Parallel()

	if got := (SettingSpec{Key: "source_url"}).EnvName(); got != "COLLECTOR_SETTING_SOURCE_URL" {
		t.Errorf("EnvName = %q", got)
	}
}

func TestLocalizedText_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
	}{
		{name: "plain string", in: `"Source URL"`},
		{name: "translations", in: `{"de":"Quell-URL","en":"Source URL"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var text LocalizedText
			if err := json.Unmarshal([]byte(tt.in), &text); err != nil {
				t.Fatal(err)
			}

			out, err := json.Marshal(text)
			if err != nil {
				t.Fatal(err)
			}

			if string(out) != tt.in {
				t.Errorf("round trip = %s, want %s", out, tt.in)
			}
		})
	}
}

func TestLocalizedText_RejectsMixedForms(t *testing.T) {
	t.Parallel()

	err := LocalizedText{"": "x", "en": "y"}.validate(10)
	checkErr(t, err, "not both")
}

func TestUnsupportedVersionIsWrapped(t *testing.T) {
	t.Parallel()

	_, err := DecodeOutput([]byte(`{"contractVersion":0}`))
	if !errors.Is(err, errUnsupportedVersion) {
		t.Errorf("err = %v, want it to wrap errUnsupportedVersion", err)
	}
}

// checkErr fails t unless err is nil when wantErr is empty, or contains
// wantErr otherwise.
func checkErr(t *testing.T, err error, wantErr string) {
	t.Helper()

	switch {
	case wantErr == "" && err != nil:
		t.Fatalf("unexpected error: %v", err)
	case wantErr != "" && err == nil:
		t.Fatalf("expected error containing %q, got nil", wantErr)
	case wantErr != "" && !strings.Contains(err.Error(), wantErr):
		t.Fatalf("error = %q, want it to contain %q", err, wantErr)
	}
}
