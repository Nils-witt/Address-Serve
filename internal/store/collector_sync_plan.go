package store

import (
	"fmt"
	"math"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

// plannedAddress is a valid, unique address of a collector output.
type plannedAddress struct {
	index int
	addr  contract.Address
}

// streetGroup is the addresses of one street in a collector output.
type streetGroup struct {
	key        contract.StreetKey
	externalID *string
	addresses  []plannedAddress
	// skip is set when the street can't be synced; its addresses are then
	// skipped too.
	skip bool
	// streetID is the stored street the group was matched to or created as.
	streetID uuid.UUID
}

// numberKey identifies a house number on its street. A missing addition is
// the empty string, which the contract doesn't allow as an addition.
type numberKey struct {
	number   int
	addition string
}

func addressNumberKey(a contract.Address) numberKey {
	return numberKey{number: a.Number, addition: deref(a.NumberAddition)}
}

func describeStreet(k contract.StreetKey) string {
	return fmt.Sprintf("%s (%s, %s, %s)", k.Name, k.District, k.City, k.Country)
}

func describeAddress(a contract.Address) string {
	return fmt.Sprintf("%s %d%s (%s, %s, %s)", a.Street, a.Number, deref(a.NumberAddition), a.District, a.City, a.Country)
}

func (s *syncer) skipAddress(index int, a contract.Address, reason string) {
	s.stats.HouseNumbers.Skipped++
	s.issues.add(SyncIssue{Kind: IssueSkipped, Entity: EntityAddress, Index: &index, Key: describeAddress(a), Reason: reason})
}

// planAddresses drops invalid and repeated addresses and groups the rest by
// street, skipping streets whose external IDs disagree.
func (s *syncer) planAddresses(addresses []contract.Address) []*streetGroup {
	type naturalKey struct {
		street contract.StreetKey
		number numberKey
	}

	seenNatural := make(map[naturalKey]bool, len(addresses))
	seenExternal := map[string]bool{}
	byKey := map[contract.StreetKey]*streetGroup{}

	var groups []*streetGroup

	for i, a := range addresses {
		if err := a.Validate(); err != nil {
			s.skipAddress(i, a, err.Error())
			continue
		}

		nk := naturalKey{a.StreetKey(), addressNumberKey(a)}
		if seenNatural[nk] || (a.HouseNumberExternalID != nil && seenExternal[*a.HouseNumberExternalID]) {
			s.skipAddress(i, a, "duplicate of an earlier address")
			continue
		}

		seenNatural[nk] = true

		if a.HouseNumberExternalID != nil {
			seenExternal[*a.HouseNumberExternalID] = true
		}

		g := byKey[nk.street]
		if g == nil {
			g = &streetGroup{key: nk.street, externalID: a.StreetExternalID}
			byKey[nk.street] = g
			groups = append(groups, g)
		}

		g.addresses = append(g.addresses, plannedAddress{index: i, addr: a})
	}

	s.skipInconsistentStreets(groups)

	return groups
}

// skipInconsistentStreets skips every group whose addresses disagree on the
// street's external ID, and every group sharing an external ID with a
// differently named one: either way, which stored street is meant is
// ambiguous.
func (s *syncer) skipInconsistentStreets(groups []*streetGroup) {
	byExternalID := map[string][]*streetGroup{}

	for _, g := range groups {
		for _, a := range g.addresses {
			if deref(a.addr.StreetExternalID) != deref(g.externalID) {
				g.skip = true
			}
		}

		if g.externalID != nil {
			byExternalID[*g.externalID] = append(byExternalID[*g.externalID], g)
		}
	}

	for _, shared := range byExternalID {
		if len(shared) > 1 {
			for _, g := range shared {
				g.skip = true
			}
		}
	}

	for _, g := range groups {
		if g.skip {
			for _, a := range g.addresses {
				s.skipAddress(a.index, a.addr, "inconsistent street: its addresses disagree on streetExternalId, "+
					"or the streetExternalId is also used for a differently named street")
			}
		}
	}
}

// medoid returns the coordinates of the address closest to the centroid of
// all of them, so a street's point always lies on one of its addresses.
func medoid(addresses []plannedAddress) (lat, lon float64) {
	var sumLat, sumLon float64

	for _, a := range addresses {
		sumLat += *a.addr.Latitude
		sumLon += *a.addr.Longitude
	}

	n := float64(len(addresses))
	centerLat, centerLon := sumLat/n, sumLon/n
	// Scales longitude differences to the latitude, so distances compare
	// correctly away from the equator; good enough at a street's scale.
	scale := math.Cos(centerLat * math.Pi / 180)
	best := math.Inf(1)

	for _, a := range addresses {
		dLat := *a.addr.Latitude - centerLat
		dLon := (*a.addr.Longitude - centerLon) * scale

		if d := dLat*dLat + dLon*dLon; d < best {
			best, lat, lon = d, *a.addr.Latitude, *a.addr.Longitude
		}
	}

	return lat, lon
}

// plannedGeoObject is a valid, unique geo object of a collector output.
type plannedGeoObject struct {
	index int
	geo   contract.GeoObject
}

type groupName struct {
	group, name string
}

func describeGeoObject(g contract.GeoObject) string {
	if g.ExternalID != nil {
		return fmt.Sprintf("%s/%s (%s)", g.Group, g.Name, *g.ExternalID)
	}

	return g.Group + "/" + g.Name
}

func (s *syncer) skipGeoObject(index int, g contract.GeoObject, reason string) {
	s.stats.GeoObjects.Skipped++
	s.issues.add(SyncIssue{Kind: IssueSkipped, Entity: EntityGeoObject, Index: &index, Key: describeGeoObject(g), Reason: reason})
}

// planGeoObjects drops invalid and repeated geo objects.
func (s *syncer) planGeoObjects(geoObjects []contract.GeoObject) []plannedGeoObject {
	seenNames := make(map[groupName]bool, len(geoObjects))
	seenExternal := map[string]bool{}

	var planned []plannedGeoObject

	for i, g := range geoObjects {
		if err := g.Validate(); err != nil {
			s.skipGeoObject(i, g, err.Error())
			continue
		}

		gn := groupName{g.Group, g.Name}
		if seenNames[gn] || (g.ExternalID != nil && seenExternal[*g.ExternalID]) {
			s.skipGeoObject(i, g, "duplicate of an earlier geo object")
			continue
		}

		seenNames[gn] = true

		if g.ExternalID != nil {
			seenExternal[*g.ExternalID] = true
		}

		planned = append(planned, plannedGeoObject{index: i, geo: g})
	}

	return planned
}
