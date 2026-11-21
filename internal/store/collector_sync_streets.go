package store

import (
	"slices"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

// streetIndex finds streets by ID, external ID and natural key.
type streetIndex struct {
	byID  map[uuid.UUID]*Street
	byExt map[string]*Street
	byKey map[contract.StreetKey]*Street
}

func newStreetIndex(rows []Street) *streetIndex {
	ix := &streetIndex{byID: map[uuid.UUID]*Street{}, byExt: map[string]*Street{}, byKey: map[contract.StreetKey]*Street{}}
	for i := range rows {
		ix.add(&rows[i])
	}

	return ix
}

func (ix *streetIndex) add(st *Street) {
	ix.byID[st.ID] = st
	ix.byKey[streetKeyOf(st)] = st

	if st.ExternalID != nil {
		ix.byExt[*st.ExternalID] = st
	}
}

func (ix *streetIndex) remove(st *Street) {
	delete(ix.byID, st.ID)
	delete(ix.byKey, streetKeyOf(st))

	if st.ExternalID != nil {
		delete(ix.byExt, *st.ExternalID)
	}
}

func streetKeyOf(st *Street) contract.StreetKey {
	return contract.StreetKey{Name: st.Name, District: st.District, City: st.City, Country: st.Country}
}

// syncStreets matches each street of the output to a stored street of the
// collector, updating it, or creates it. Streets owned by others are
// conflicts, which skip the street with all its addresses.
func (s *syncer) syncStreets(groups []*streetGroup) error {
	var owned []Street
	if err := s.tx.Where("collector_id = ?", s.collectorID).Find(&owned).Error; err != nil {
		return err
	}

	s.ownedStreets = newStreetIndex(owned)
	s.claimedStreets = map[uuid.UUID]bool{}
	s.keptStreets = map[uuid.UUID]bool{}

	foreignStreets, err := s.loadForeignStreets(groups)
	if err != nil {
		return err
	}

	var inserts []*streetGroup

	for _, g := range groups {
		if g.skip {
			continue
		}

		target, conflict, reason := s.matchStreet(g, foreignStreets)

		switch {
		case reason != "":
			s.conflictStreet(g, conflict, reason)

			// The entry still names this street of the collector, so it is
			// kept as it is rather than deleted as stale.
			if target != nil {
				s.claimedStreets[target.ID] = true
				s.keptStreets[target.ID] = true
			}
		case target == nil:
			inserts = append(inserts, g)
		default:
			if err := s.updateStreet(target, g); err != nil {
				return err
			}
		}
	}

	return s.insertStreets(inserts)
}

// loadForeignStreets loads the streets not owned by the collector that share
// an external ID or a natural key with a street of the output.
func (s *syncer) loadForeignStreets(groups []*streetGroup) (*streetIndex, error) {
	var keys, externalIDs []any

	for _, g := range groups {
		if g.skip {
			continue
		}

		keys = append(keys, []any{g.key.Name, g.key.District, g.key.City, g.key.Country})

		if g.externalID != nil {
			externalIDs = append(externalIDs, *g.externalID)
		}
	}

	var rows []Street
	if err := findIn(s.tx, &rows, "(name, district, city, country)", keys, foreign, s.collectorID); err != nil {
		return nil, err
	}

	if err := findIn(s.tx, &rows, "external_id", externalIDs, foreign, s.collectorID); err != nil {
		return nil, err
	}

	return newStreetIndex(rows), nil
}

// matchStreet finds the collector's stored street for g, by external ID and
// then by natural key. It returns no street for a new one, and a reason and
// the conflicting row, if any, when g can't be synced; target is then the
// collector's street that g still names, if any.
func (s *syncer) matchStreet(g *streetGroup, foreignStreets *streetIndex) (target, conflict *Street, reason string) {
	if g.externalID != nil {
		target = s.ownedStreets.byExt[*g.externalID]
		if f := foreignStreets.byExt[*g.externalID]; target == nil && f != nil {
			return nil, f, "streetExternalId is used by a street of " + ownerName(f.CollectorID)
		}
	}

	if target == nil {
		target = s.ownedStreets.byKey[g.key]

		switch f := foreignStreets.byKey[g.key]; {
		case target == nil && f != nil:
			return nil, f, "the street belongs to " + ownerName(f.CollectorID)
		case target == nil:
			return nil, nil, ""
		case target.ExternalID != nil && g.externalID != nil && *target.ExternalID != *g.externalID:
			return target, target, "the stored street has a different externalId: " + *target.ExternalID
		}
	}

	if s.claimedStreets[target.ID] {
		return nil, target, "matches the same stored street as an earlier street of the output"
	}

	if conflict, reason := s.checkRename(target, g.key, foreignStreets); reason != "" {
		return target, conflict, reason
	}

	return target, nil, ""
}

// checkRename reports why target can't take key as its new name, if it
// can't.
func (s *syncer) checkRename(target *Street, key contract.StreetKey, foreignStreets *streetIndex) (*Street, string) {
	if streetKeyOf(target) == key {
		return nil, ""
	}

	if o := s.ownedStreets.byKey[key]; o != nil {
		return o, "can't rename the street: another street of this collector has this name"
	}

	if f := foreignStreets.byKey[key]; f != nil {
		return f, "can't rename the street: its new name belongs to " + ownerName(f.CollectorID)
	}

	return nil, ""
}

func (s *syncer) conflictStreet(g *streetGroup, row *Street, reason string) {
	g.skip = true
	s.stats.Streets.Conflicts++
	s.stats.HouseNumbers.Skipped += len(g.addresses)

	issue := SyncIssue{Kind: IssueConflict, Entity: EntityStreet, Key: describeStreet(g.key), Reason: reason}
	if row != nil {
		issue.RowID = &row.ID
	}

	s.issues.add(issue)
}

// updateStreet makes target match g.
func (s *syncer) updateStreet(target *Street, g *streetGroup) error {
	lat, lon := medoid(g.addresses)
	changes := map[string]any{}

	if streetKeyOf(target) != g.key {
		changes["name"], changes["district"], changes["city"], changes["country"] =
			g.key.Name, g.key.District, g.key.City, g.key.Country
	}

	if target.Latitude != lat || target.Longitude != lon {
		changes["latitude"], changes["longitude"] = lat, lon
	}

	if deref(target.ExternalID) != deref(g.externalID) {
		changes["external_id"] = g.externalID
	}

	s.claimedStreets[target.ID] = true
	g.streetID = target.ID

	if len(changes) == 0 {
		s.stats.Streets.Unchanged++
		return nil
	}

	if err := s.tx.Model(&Street{}).Where("id = ?", target.ID).Updates(changes).Error; err != nil {
		return err
	}

	s.ownedStreets.remove(target)
	target.Name, target.District, target.City, target.Country = g.key.Name, g.key.District, g.key.City, g.key.Country
	target.Latitude, target.Longitude, target.ExternalID = lat, lon, g.externalID
	s.ownedStreets.add(target)
	s.stats.Streets.Updated++

	return nil
}

func (s *syncer) insertStreets(groups []*streetGroup) error {
	if len(groups) == 0 {
		return nil
	}

	rows := make([]Street, len(groups))

	for i, g := range groups {
		lat, lon := medoid(g.addresses)
		rows[i] = Street{
			Name: g.key.Name, District: g.key.District, City: g.key.City, Country: g.key.Country,
			Latitude: lat, Longitude: lon, CollectorID: &s.collectorID, ExternalID: g.externalID,
		}
	}

	if err := s.tx.CreateInBatches(&rows, insertBatchSize).Error; err != nil {
		return err
	}

	for i, g := range groups {
		g.streetID = rows[i].ID
		s.claimedStreets[rows[i].ID] = true
		s.ownedStreets.add(&rows[i])
	}

	s.stats.Streets.Created += len(rows)

	return nil
}

// removeStaleStreets deletes the collector's streets that the output no
// longer has. One that still carries rows of others is released instead:
// deleting it would cascade to them.
func (s *syncer) removeStaleStreets() error {
	var stale []uuid.UUID

	for id := range s.ownedStreets.byID {
		if !s.claimedStreets[id] {
			stale = append(stale, id)
		}
	}

	if len(stale) == 0 {
		return nil
	}

	slices.SortFunc(stale, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })

	held, err := s.withForeignRows(stale, "house_numbers", "street_id")
	if err != nil {
		return err
	}

	heldByGeo, err := s.withForeignRows(stale, "geo_objects", "street_id")
	if err != nil {
		return err
	}

	var deletable, released []uuid.UUID

	for _, id := range stale {
		if held[id] || heldByGeo[id] {
			released = append(released, id)
			st := s.ownedStreets.byID[id]
			s.issues.add(SyncIssue{
				Kind: IssueReleased, Entity: EntityStreet, Key: describeStreet(streetKeyOf(st)), RowID: &st.ID,
				Reason: "no longer returned, but other rows are on it; it is now a manual street",
			})
		} else {
			deletable = append(deletable, id)
		}
	}

	if err := s.release(&Street{}, released); err != nil {
		return err
	}

	s.stats.Streets.Released += len(released)
	deleted, err := s.deleteIDs(&Street{}, deletable)
	s.stats.Streets.Deleted += deleted

	return err
}
