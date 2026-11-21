// Command example is a reference collector for address-serv: it implements
// contract version 1 (docs/collectors/contract.md) and returns fixed sample
// data. It uses only the standard library, so it can be copied as a starting
// point outside this repository.
//
// Build it as a static binary into the collectors folder:
//
//	CGO_ENABLED=0 go build -o collectors/example ./examples/collectors/example
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
)

// The documents of contract version 1. Optional fields are pointers or
// omitempty, so they are left out rather than sent empty.
type (
	description struct {
		ContractVersion int               `json:"contractVersion"`
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		Description     map[string]string `json:"description,omitempty"`
		Settings        []setting         `json:"settings"`
	}

	setting struct {
		Key         string            `json:"key"`
		Label       map[string]string `json:"label"`
		Description map[string]string `json:"description,omitempty"`
		Type        string            `json:"type"`
		Required    bool              `json:"required,omitempty"`
		Default     any               `json:"default,omitempty"`
	}

	output struct {
		ContractVersion int         `json:"contractVersion"`
		Addresses       []address   `json:"addresses"`
		GeoObjects      []geoObject `json:"geoObjects"`
	}

	address struct {
		StreetExternalID      string  `json:"streetExternalId,omitempty"`
		HouseNumberExternalID string  `json:"houseNumberExternalId,omitempty"`
		Street                string  `json:"street"`
		District              string  `json:"district"`
		City                  string  `json:"city"`
		Country               string  `json:"country"`
		Number                int     `json:"number"`
		NumberAddition        *string `json:"numberAddition,omitempty"`
		Postcode              string  `json:"postcode"`
		Latitude              float64 `json:"latitude"`
		Longitude             float64 `json:"longitude"`
	}

	addressRef struct {
		Street   string `json:"street"`
		District string `json:"district"`
		City     string `json:"city"`
		Country  string `json:"country"`
		Number   *int   `json:"number,omitempty"`
	}

	geoObject struct {
		ExternalID string      `json:"externalId,omitempty"`
		Group      string      `json:"group"`
		Name       string      `json:"name"`
		Latitude   float64     `json:"latitude"`
		Longitude  float64     `json:"longitude"`
		Address    *addressRef `json:"address,omitempty"`
	}
)

const (
	defaultCity  = "Exampletown"
	defaultCount = 3
)

func describe() description {
	return description{
		ContractVersion: 1,
		Name:            "Example",
		Version:         "1.0.0",
		Description: map[string]string{
			"en": "Fixed sample addresses and hydrants, for trying out collectors",
			"de": "Feste Beispieladressen und Hydranten, zum Ausprobieren von Collectors",
		},
		Settings: []setting{
			{
				Key: "city", Type: "string", Default: defaultCity,
				Label: map[string]string{"en": "City", "de": "Stadt"},
			},
			{
				Key: "count", Type: "integer", Default: defaultCount,
				Label:       map[string]string{"en": "House numbers per street", "de": "Hausnummern pro Straße"},
				Description: map[string]string{"en": "Between 1 and 50", "de": "Zwischen 1 und 50"},
			},
			{
				Key: "with_hydrants", Type: "boolean", Default: true,
				Label: map[string]string{"en": "Include hydrants", "de": "Hydranten einschließen"},
			},
		},
	}
}

// config is the settings address-serv passes in COLLECTOR_SETTING_* variables.
// Settings with a default always arrive, but a collector should still cope
// with their absence, e.g. when it is run by hand.
type config struct {
	city         string
	count        int
	withHydrants bool
}

func configFromEnv(getenv func(string) string) (config, error) {
	cfg := config{city: defaultCity, count: defaultCount, withHydrants: true}

	if v := getenv("COLLECTOR_SETTING_CITY"); v != "" {
		cfg.city = v
	}

	if v := getenv("COLLECTOR_SETTING_COUNT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 50 {
			return config{}, fmt.Errorf("count must be between 1 and 50, got %q", v)
		}

		cfg.count = n
	}

	if v := getenv("COLLECTOR_SETTING_WITH_HYDRANTS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return config{}, fmt.Errorf("with_hydrants must be true or false, got %q", v)
		}

		cfg.withHydrants = b
	}

	return cfg, nil
}

// collect builds the sample data. A real collector would fetch it from its
// source here.
func collect(cfg config) output {
	out := output{ContractVersion: 1, Addresses: []address{}, GeoObjects: []geoObject{}}

	for s, street := range []string{"Main Street", "Station Road"} {
		for n := 1; n <= cfg.count; n++ {
			out.Addresses = append(out.Addresses, address{
				// Stable IDs let address-serv follow renames in the source.
				StreetExternalID:      fmt.Sprintf("example-street-%d", s),
				HouseNumberExternalID: fmt.Sprintf("example-hn-%d-%d", s, n),
				Street:                street,
				District:              "Center",
				City:                  cfg.city,
				Country:               "Germany",
				Number:                n,
				Postcode:              "12345",
				Latitude:              50.73 + float64(s)*0.002,
				Longitude:             7.10 + float64(n)*0.0003,
			})
		}
	}

	if cfg.withHydrants {
		first := 1
		out.GeoObjects = append(out.GeoObjects,
			// Placed at an address.
			geoObject{
				ExternalID: "example-hydrant-1", Group: "hydrant", Name: "Hydrant 1",
				Latitude: 50.7301, Longitude: 7.1003,
				Address: &addressRef{Street: "Main Street", District: "Center", City: cfg.city, Country: "Germany", Number: &first},
			},
			// Placed at the nearest house number by address-serv.
			geoObject{ExternalID: "example-hydrant-2", Group: "hydrant", Name: "Hydrant 2", Latitude: 50.7321, Longitude: 7.1006},
		)
	}

	return out
}

func run(args []string, getenv func(string) string, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: %s describe|collect", os.Args[0])
	}

	enc := json.NewEncoder(stdout)

	switch args[0] {
	case "describe":
		return enc.Encode(describe())
	case "collect":
		cfg, err := configFromEnv(getenv)
		if err != nil {
			return err
		}

		// stderr is the run's log in address-serv.
		log.Printf("collecting %d house numbers per street in %s", cfg.count, cfg.city)

		return enc.Encode(collect(cfg))
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func main() {
	log.SetFlags(0)

	// A non-zero exit fails the run, and nothing is applied.
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
