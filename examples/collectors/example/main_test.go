package main

import (
	"bytes"
	"testing"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

// TestRun checks the example's output against the contract the app enforces,
// so the reference can't drift from it.
func TestRun(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run([]string{"describe"}, func(string) string { return "" }, &out); err != nil {
		t.Fatal(err)
	}

	d, err := contract.DecodeDescription(out.Bytes())
	if err != nil {
		t.Fatalf("describe output %s: %v", out.Bytes(), err)
	}

	if len(d.Settings) != 3 {
		t.Errorf("settings = %+v", d.Settings)
	}

	tests := []struct {
		name          string
		env           map[string]string
		wantAddresses int
		wantGeo       int
	}{
		{name: "defaults", wantAddresses: 6, wantGeo: 2},
		{name: "settings", env: map[string]string{"COLLECTOR_SETTING_COUNT": "5", "COLLECTOR_SETTING_WITH_HYDRANTS": "false"}, wantAddresses: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			if err := run([]string{"collect"}, func(k string) string { return tt.env[k] }, &out); err != nil {
				t.Fatal(err)
			}

			o, err := contract.DecodeOutput(out.Bytes())
			if err != nil {
				t.Fatal(err)
			}

			if len(o.Addresses) != tt.wantAddresses || len(o.GeoObjects) != tt.wantGeo {
				t.Errorf("got %d addresses and %d geo objects, want %d and %d",
					len(o.Addresses), len(o.GeoObjects), tt.wantAddresses, tt.wantGeo)
			}

			for i, a := range o.Addresses {
				if err := a.Validate(); err != nil {
					t.Errorf("addresses[%d]: %v", i, err)
				}
			}

			for i, g := range o.GeoObjects {
				if err := g.Validate(); err != nil {
					t.Errorf("geoObjects[%d]: %v", i, err)
				}
			}
		})
	}
}

func TestRun_RejectsBadInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{name: "no command", args: nil},
		{name: "unknown command", args: []string{"sync"}},
		{name: "count out of range", args: []string{"collect"}, env: map[string]string{"COLLECTOR_SETTING_COUNT": "0"}},
		{name: "bad boolean", args: []string{"collect"}, env: map[string]string{"COLLECTOR_SETTING_WITH_HYDRANTS": "maybe"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := run(tt.args, func(k string) string { return tt.env[k] }, &bytes.Buffer{}); err == nil {
				t.Error("run succeeded")
			}
		})
	}
}
