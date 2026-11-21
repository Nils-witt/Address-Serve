package store

import (
	"fmt"

	"github.com/google/uuid"
)

// hnKey identifies a house number by its street and number.
type hnKey struct {
	streetID uuid.UUID
	number   numberKey
}

func keyOfHouseNumber(h *HouseNumber) hnKey {
	return hnKey{streetID: h.StreetID, number: numberKey{number: h.Number, addition: deref(h.NumberAddition)}}
}

// houseNumberSync holds the state of the house-number phase of a sync.
type houseNumberSync struct {
	*syncer

	// byKey holds every stored house number on the collector's streets, and
	// the collector's own wherever they are, by street and number.
	byKey      map[hnKey]*HouseNumber
	ownedByExt map[string]*HouseNumber
	// foreignByExt holds house numbers not owned by the collector that share
	// an external ID with an address of the output.
	foreignByExt map[string]*HouseNumber
	owned        []*HouseNumber
	claimed      map[uuid.UUID]bool
}

// houseNumberOp is what to do for one address: update target, or insert a
// house number if target is nil.
type houseNumberOp struct {
	group  *streetGroup
	addr   plannedAddress
	target *HouseNumber
	adopt  bool
}

func (s *syncer) ownsHouseNumber(h *HouseNumber) bool {
	return h.CollectorID != nil && *h.CollectorID == s.collectorID
}

// syncHouseNumbers syncs the addresses of the streets that syncStreets
// matched or created. A manual house number with an address's number on one
// of the collector's streets is adopted; one owned by another collector is a
// conflict.
func (s *syncer) syncHouseNumbers(groups []*streetGroup) error {
	hs, err := s.loadHouseNumbers(groups)
	if err != nil {
		return err
	}

	var ops []houseNumberOp

	for _, g := range groups {
		if g.skip {
			continue
		}

		for _, a := range g.addresses {
			op, conflict, reason := hs.match(g, a)
			if reason != "" {
				hs.conflict(a, conflict, reason)

				// The entry still names this house number of the
				// collector, so it stays rather than being deleted as stale.
				if op.target != nil && hs.ownsHouseNumber(op.target) {
					hs.claimed[op.target.ID] = true
				}

				continue
			}

			if op.target != nil {
				hs.claimed[op.target.ID] = true
			}

			ops = append(ops, op)
		}
	}

	// Deleting stale house numbers first frees their numbers for the ones
	// that move onto them.
	if err := hs.removeStale(); err != nil {
		return err
	}

	var inserts []houseNumberOp

	for _, op := range ops {
		if op.target == nil {
			inserts = append(inserts, op)
			continue
		}

		if err := hs.update(op); err != nil {
			return err
		}
	}

	return hs.insert(inserts)
}

func (s *syncer) loadHouseNumbers(groups []*streetGroup) (*houseNumberSync, error) {
	var rows []HouseNumber

	err := s.tx.Where("collector_id = ? OR street_id IN (SELECT id FROM streets WHERE collector_id = ?)",
		s.collectorID, s.collectorID).Find(&rows).Error
	if err != nil {
		return nil, err
	}

	hs := &houseNumberSync{
		syncer:       s,
		byKey:        make(map[hnKey]*HouseNumber, len(rows)),
		ownedByExt:   map[string]*HouseNumber{},
		foreignByExt: map[string]*HouseNumber{},
		claimed:      map[uuid.UUID]bool{},
	}

	for i := range rows {
		h := &rows[i]
		hs.byKey[keyOfHouseNumber(h)] = h

		if s.ownsHouseNumber(h) {
			hs.owned = append(hs.owned, h)

			if h.ExternalID != nil {
				hs.ownedByExt[*h.ExternalID] = h
			}
		}
	}

	var externalIDs []any

	for _, g := range groups {
		for _, a := range g.addresses {
			if id := a.addr.HouseNumberExternalID; !g.skip && id != nil && hs.ownedByExt[*id] == nil {
				externalIDs = append(externalIDs, *id)
			}
		}
	}

	var foreignRows []HouseNumber
	if err := findIn(s.tx, &foreignRows, "external_id", externalIDs, foreign, s.collectorID); err != nil {
		return nil, err
	}

	for i := range foreignRows {
		hs.foreignByExt[*foreignRows[i].ExternalID] = &foreignRows[i]
	}

	return hs, nil
}

// match finds the stored house number for a, by external ID and then by
// number on the matched street. It returns a reason and the conflicting row
// when a can't be synced.
func (hs *houseNumberSync) match(g *streetGroup, a plannedAddress) (op houseNumberOp, conflict *HouseNumber, reason string) {
	op = houseNumberOp{group: g, addr: a}
	ext := a.addr.HouseNumberExternalID

	if ext != nil {
		op.target = hs.ownedByExt[*ext]
		if f := hs.foreignByExt[*ext]; op.target == nil && f != nil {
			return op, f, "houseNumberExternalId is used by a house number of " + ownerName(f.CollectorID)
		}
	}

	if op.target == nil {
		c := hs.byKey[hnKey{streetID: g.streetID, number: addressNumberKey(a.addr)}]

		switch {
		case c == nil:
			return op, nil, ""
		case hs.ownsHouseNumber(c) && c.ExternalID != nil && ext != nil && *c.ExternalID != *ext:
			op.target = c
			return op, c, "the stored house number has a different externalId: " + *c.ExternalID
		case hs.ownsHouseNumber(c):
			op.target = c
		case c.CollectorID == nil:
			op.target, op.adopt = c, true
		default:
			return op, c, "the house number belongs to " + ownerName(c.CollectorID)
		}
	}

	if hs.claimed[op.target.ID] {
		return op, op.target, "matches the same stored house number as an earlier address"
	}

	return op, nil, ""
}

func (hs *houseNumberSync) conflict(a plannedAddress, row *HouseNumber, reason string) {
	hs.stats.HouseNumbers.Conflicts++

	issue := SyncIssue{Kind: IssueConflict, Entity: EntityAddress, Index: &a.index, Key: describeAddress(a.addr), Reason: reason}
	if row != nil {
		issue.RowID = &row.ID
	}

	hs.issues.add(issue)
}

func (hs *houseNumberSync) describe(h *HouseNumber) string {
	street := "unknown street"
	if st := hs.ownedStreets.byID[h.StreetID]; st != nil {
		street = describeStreet(streetKeyOf(st))
	}

	return fmt.Sprintf("%d%s on %s", h.Number, deref(h.NumberAddition), street)
}

// removeStale deletes the collector's house numbers that the output no
// longer has, except those that geo objects of others point to: deleting
// those would change them, so they are released instead.
func (hs *houseNumberSync) removeStale() error {
	var stale []uuid.UUID

	for _, h := range hs.owned {
		// The house numbers of a kept street were skipped with it, and stay.
		if hs.keptStreets[h.StreetID] {
			hs.claimed[h.ID] = true
		}

		if !hs.claimed[h.ID] {
			stale = append(stale, h.ID)
		}
	}

	held, err := hs.withForeignRows(stale, "geo_objects", "house_number_id")
	if err != nil {
		return err
	}

	var deletable, released []uuid.UUID

	for _, h := range hs.owned {
		switch {
		case hs.claimed[h.ID]:
		case held[h.ID]:
			released = append(released, h.ID)
			h.CollectorID, h.ExternalID = nil, nil
			hs.issues.add(SyncIssue{
				Kind: IssueReleased, Entity: EntityHouseNumber, Key: hs.describe(h), RowID: &h.ID,
				Reason: "no longer returned, but geo objects of others point to it; it is now a manual house number",
			})
		default:
			deletable = append(deletable, h.ID)
			delete(hs.byKey, keyOfHouseNumber(h))
		}
	}

	if err := hs.release(&HouseNumber{}, released); err != nil {
		return err
	}

	hs.stats.HouseNumbers.Released += len(released)
	deleted, err := hs.deleteIDs(&HouseNumber{}, deletable)
	hs.stats.HouseNumbers.Deleted += deleted

	return err
}

// update makes op.target match its address, which may move it to another
// number or, when matched by external ID, another street of the collector.
func (hs *houseNumberSync) update(op houseNumberOp) error {
	h, a := op.target, op.addr.addr
	newKey := hnKey{streetID: op.group.streetID, number: addressNumberKey(a)}

	if o := hs.byKey[newKey]; o != nil && o != h {
		hs.conflict(op.addr, o, "can't move the house number: its new number is taken by a house number of "+
			ownerName(o.CollectorID))

		return nil
	}

	changes := houseNumberChanges(h, op.group.streetID, a.Number, a.NumberAddition, a.Postcode,
		*a.Latitude, *a.Longitude, a.HouseNumberExternalID)
	if op.adopt {
		changes["collector_id"] = hs.collectorID
	}

	switch {
	case op.adopt:
		hs.stats.HouseNumbers.Adopted++
		hs.issues.add(SyncIssue{
			Kind: IssueAdopted, Entity: EntityHouseNumber, Index: &op.addr.index, Key: describeAddress(a), RowID: &h.ID,
			Reason: "a manually created house number on a street of this collector; the collector now owns it",
		})
	case len(changes) == 0:
		hs.stats.HouseNumbers.Unchanged++
		return nil
	default:
		hs.stats.HouseNumbers.Updated++
	}

	if err := hs.tx.Model(&HouseNumber{}).Where("id = ?", h.ID).Updates(changes).Error; err != nil {
		return err
	}

	delete(hs.byKey, keyOfHouseNumber(h))
	h.StreetID, h.Number, h.NumberAddition = op.group.streetID, a.Number, a.NumberAddition
	hs.byKey[newKey] = h

	return nil
}

// houseNumberChanges returns the columns of h that differ from the given
// values.
func houseNumberChanges(
	h *HouseNumber, streetID uuid.UUID, number int, addition *string, postcode string, lat, lon float64, ext *string,
) map[string]any {
	changes := map[string]any{}

	if h.StreetID != streetID {
		changes["street_id"] = streetID
	}

	if h.Number != number || deref(h.NumberAddition) != deref(addition) {
		changes["number"], changes["number_addition"] = number, addition
	}

	if h.Postcode != postcode {
		changes["postcode"] = postcode
	}

	if h.Latitude != lat || h.Longitude != lon {
		changes["latitude"], changes["longitude"] = lat, lon
	}

	if deref(h.ExternalID) != deref(ext) {
		changes["external_id"] = ext
	}

	return changes
}

func (hs *houseNumberSync) insert(ops []houseNumberOp) error {
	rows := make([]HouseNumber, 0, len(ops))

	for _, op := range ops {
		a := op.addr.addr

		key := hnKey{streetID: op.group.streetID, number: addressNumberKey(a)}
		if o := hs.byKey[key]; o != nil {
			hs.conflict(op.addr, o, "the number is taken by a house number of "+ownerName(o.CollectorID))
			continue
		}

		rows = append(rows, HouseNumber{
			StreetID: op.group.streetID, Number: a.Number, NumberAddition: a.NumberAddition, Postcode: a.Postcode,
			Latitude: *a.Latitude, Longitude: *a.Longitude, CollectorID: &hs.collectorID, ExternalID: a.HouseNumberExternalID,
		})
		hs.byKey[key] = &rows[len(rows)-1]
	}

	if len(rows) == 0 {
		return nil
	}

	if err := hs.tx.CreateInBatches(&rows, insertBatchSize).Error; err != nil {
		return err
	}

	hs.stats.HouseNumbers.Created += len(rows)

	return nil
}
