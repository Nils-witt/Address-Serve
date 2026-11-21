package store

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

// geoLink is where a geo object is attached.
type geoLink struct {
	streetID      uuid.UUID
	houseNumberID *uuid.UUID
}

// geoObjectSync holds the state of the geo-object phase of a sync.
type geoObjectSync struct {
	*syncer

	ownedByExt     map[string]*GeoObject
	ownedByName    map[groupName]*GeoObject
	foreignByExt   map[string]*GeoObject
	foreignByName  map[groupName]*GeoObject
	owned          []*GeoObject
	claimed        map[uuid.UUID]bool
	streetsByExt   map[string]*Street
	streetsByKey   map[contract.StreetKey]*Street
	numbersByExt   map[string]*HouseNumber
	numbersByKey   map[hnKey]*HouseNumber
	maxDistanceMsg string
}

type geoObjectOp struct {
	planned plannedGeoObject
	link    geoLink
	target  *GeoObject
}

// syncGeoObjects places each geo object of the output, then matches it to a
// stored geo object of the collector by external ID or group and name.
func (s *syncer) syncGeoObjects(planned []plannedGeoObject) error {
	gs, err := s.loadGeoObjects(planned)
	if err != nil {
		return err
	}

	if err := gs.loadReferences(planned); err != nil {
		return err
	}

	var ops []geoObjectOp

	for _, p := range planned {
		link, reason, err := gs.resolve(p.geo)
		if err != nil {
			return err
		}

		if reason != "" {
			s.skipGeoObject(p.index, p.geo, reason)
			continue
		}

		op, conflict, reason := gs.match(p, link)
		if reason != "" {
			gs.conflict(p, conflict, reason)

			// The entry still names this geo object of the collector, so it
			// stays rather than being deleted as stale.
			if op.target != nil {
				gs.claimed[op.target.ID] = true
			}

			continue
		}

		if op.target != nil {
			gs.claimed[op.target.ID] = true
		}

		ops = append(ops, op)
	}

	// Deleting stale geo objects first frees their names for renamed ones.
	if err := gs.removeStale(); err != nil {
		return err
	}

	var inserts []geoObjectOp

	for _, op := range ops {
		if op.target == nil {
			inserts = append(inserts, op)
			continue
		}

		if err := gs.update(op); err != nil {
			return err
		}
	}

	return gs.insert(inserts)
}

func (s *syncer) loadGeoObjects(planned []plannedGeoObject) (*geoObjectSync, error) {
	var rows []GeoObject
	if err := s.tx.Where("collector_id = ?", s.collectorID).Find(&rows).Error; err != nil {
		return nil, err
	}

	gs := &geoObjectSync{
		syncer:        s,
		ownedByExt:    map[string]*GeoObject{},
		ownedByName:   make(map[groupName]*GeoObject, len(rows)),
		foreignByExt:  map[string]*GeoObject{},
		foreignByName: map[groupName]*GeoObject{},
		claimed:       map[uuid.UUID]bool{},
		maxDistanceMsg: fmt.Sprintf("no usable address, and no house number within %g m",
			s.opts.GeoMaxDistanceMeters),
	}

	for i := range rows {
		g := &rows[i]
		gs.owned = append(gs.owned, g)
		gs.ownedByName[groupName{g.Group, g.Name}] = g

		if g.ExternalID != nil {
			gs.ownedByExt[*g.ExternalID] = g
		}
	}

	var names, externalIDs []any

	for _, p := range planned {
		names = append(names, []any{p.geo.Group, p.geo.Name})

		if p.geo.ExternalID != nil {
			externalIDs = append(externalIDs, *p.geo.ExternalID)
		}
	}

	var foreignRows []GeoObject
	if err := findIn(s.tx, &foreignRows, "(group_name, name)", names, foreign, s.collectorID); err != nil {
		return nil, err
	}

	if err := findIn(s.tx, &foreignRows, "external_id", externalIDs, foreign, s.collectorID); err != nil {
		return nil, err
	}

	for i := range foreignRows {
		g := &foreignRows[i]
		gs.foreignByName[groupName{g.Group, g.Name}] = g

		if g.ExternalID != nil {
			gs.foreignByExt[*g.ExternalID] = g
		}
	}

	return gs, nil
}

// loadReferences loads the streets and house numbers that the geo objects'
// addresses name, whoever owns them.
func (gs *geoObjectSync) loadReferences(planned []plannedGeoObject) error {
	var streetKeys, streetExts, numberExts []any

	for _, p := range planned {
		if r := p.geo.Address; r != nil {
			k := r.StreetKey()
			streetKeys = append(streetKeys, []any{k.Name, k.District, k.City, k.Country})

			if r.StreetExternalID != nil {
				streetExts = append(streetExts, *r.StreetExternalID)
			}

			if r.HouseNumberExternalID != nil {
				numberExts = append(numberExts, *r.HouseNumberExternalID)
			}
		}
	}

	var streets []Street
	if err := findIn(gs.tx, &streets, "(name, district, city, country)", streetKeys, ""); err != nil {
		return err
	}

	if err := findIn(gs.tx, &streets, "external_id", streetExts, ""); err != nil {
		return err
	}

	ix := newStreetIndex(streets)
	gs.streetsByExt, gs.streetsByKey = ix.byExt, ix.byKey

	return gs.loadReferencedNumbers(planned, numberExts)
}

// loadReferencedNumbers loads the house numbers the geo objects' addresses
// name, on the streets loadReferences found.
func (gs *geoObjectSync) loadReferencedNumbers(planned []plannedGeoObject, numberExts []any) error {
	var numberKeys []any

	for _, p := range planned {
		if r := p.geo.Address; r != nil && r.Number != nil {
			if st := gs.referencedStreet(r); st != nil {
				numberKeys = append(numberKeys, []any{st.ID, *r.Number, deref(r.NumberAddition)})
			}
		}
	}

	var numbers []HouseNumber
	if err := findIn(gs.tx, &numbers, "(street_id, number, coalesce(number_addition, ''))", numberKeys, ""); err != nil {
		return err
	}

	if err := findIn(gs.tx, &numbers, "external_id", numberExts, ""); err != nil {
		return err
	}

	gs.numbersByExt = map[string]*HouseNumber{}
	gs.numbersByKey = make(map[hnKey]*HouseNumber, len(numbers))

	for i := range numbers {
		h := &numbers[i]
		gs.numbersByKey[keyOfHouseNumber(h)] = h

		if h.ExternalID != nil {
			gs.numbersByExt[*h.ExternalID] = h
		}
	}

	return nil
}

func (gs *geoObjectSync) referencedStreet(r *contract.AddressRef) *Street {
	if r.StreetExternalID != nil {
		if st := gs.streetsByExt[*r.StreetExternalID]; st != nil {
			return st
		}
	}

	return gs.streetsByKey[r.StreetKey()]
}

// resolve finds where g is attached: at its address if that names an
// existing street and, if it names one, house number on it; otherwise at the
// nearest house number within the configured distance. It returns a reason
// when g can't be placed.
func (gs *geoObjectSync) resolve(g contract.GeoObject) (geoLink, string, error) {
	if r := g.Address; r != nil {
		if link, ok := gs.resolveAddress(r); ok {
			return link, "", nil
		}
	}

	if gs.opts.GeoMaxDistanceMeters <= 0 {
		return geoLink{}, "no usable address", nil
	}

	h, err := nearestHouseNumber(gs.tx, *g.Latitude, *g.Longitude, gs.opts.GeoMaxDistanceMeters)
	if err != nil || h == nil {
		return geoLink{}, gs.maxDistanceMsg, err
	}

	return geoLink{streetID: h.StreetID, houseNumberID: &h.ID}, "", nil
}

func (gs *geoObjectSync) resolveAddress(r *contract.AddressRef) (geoLink, bool) {
	st := gs.referencedStreet(r)
	if st == nil {
		return geoLink{}, false
	}

	var h *HouseNumber

	switch {
	case r.HouseNumberExternalID != nil:
		h = gs.numbersByExt[*r.HouseNumberExternalID]
	case r.Number != nil:
		h = gs.numbersByKey[hnKey{streetID: st.ID, number: numberKey{*r.Number, deref(r.NumberAddition)}}]
	default:
		return geoLink{streetID: st.ID}, true
	}

	if h == nil || h.StreetID != st.ID {
		return geoLink{}, false
	}

	return geoLink{streetID: st.ID, houseNumberID: &h.ID}, true
}

// match finds the collector's stored geo object for p, by external ID and
// then by group and name. It returns a reason and the conflicting row when p
// can't be synced.
func (gs *geoObjectSync) match(p plannedGeoObject, link geoLink) (op geoObjectOp, conflict *GeoObject, reason string) {
	op = geoObjectOp{planned: p, link: link}
	g, gn := p.geo, groupName{p.geo.Group, p.geo.Name}

	if g.ExternalID != nil {
		op.target = gs.ownedByExt[*g.ExternalID]
		if f := gs.foreignByExt[*g.ExternalID]; op.target == nil && f != nil {
			return op, f, "externalId is used by a geo object of " + ownerName(f.CollectorID)
		}
	}

	if op.target == nil {
		op.target = gs.ownedByName[gn]

		switch f := gs.foreignByName[gn]; {
		case op.target == nil && f != nil:
			return op, f, "the name is used in this group by a geo object of " + ownerName(f.CollectorID)
		case op.target == nil:
			return op, nil, ""
		case op.target.ExternalID != nil && g.ExternalID != nil && *op.target.ExternalID != *g.ExternalID:
			return op, op.target, "the stored geo object with this name has a different externalId: " + *op.target.ExternalID
		}
	}

	if gs.claimed[op.target.ID] {
		return op, op.target, "matches the same stored geo object as an earlier geo object"
	}

	return op, nil, ""
}

func (gs *geoObjectSync) conflict(p plannedGeoObject, row *GeoObject, reason string) {
	gs.stats.GeoObjects.Conflicts++

	issue := SyncIssue{Kind: IssueConflict, Entity: EntityGeoObject, Index: &p.index, Key: describeGeoObject(p.geo), Reason: reason}
	if row != nil {
		issue.RowID = &row.ID
	}

	gs.issues.add(issue)
}

// removeStale deletes the collector's geo objects that the output no longer
// has. Nothing references geo objects, so none has to be released.
func (gs *geoObjectSync) removeStale() error {
	var stale []uuid.UUID

	for _, g := range gs.owned {
		if !gs.claimed[g.ID] {
			stale = append(stale, g.ID)
			delete(gs.ownedByName, groupName{g.Group, g.Name})
		}
	}

	deleted, err := gs.deleteIDs(&GeoObject{}, stale)
	gs.stats.GeoObjects.Deleted += deleted

	return err
}

func (gs *geoObjectSync) update(op geoObjectOp) error {
	g, row := op.planned.geo, op.target
	gn := groupName{g.Group, g.Name}

	if gn != (groupName{row.Group, row.Name}) {
		if o := gs.ownedByName[gn]; o != nil {
			gs.conflict(op.planned, o, "can't rename the geo object: another geo object of this collector has this name")
			return nil
		}

		if f := gs.foreignByName[gn]; f != nil {
			gs.conflict(op.planned, f, "can't rename the geo object: the name is used in this group by a geo object of "+
				ownerName(f.CollectorID))

			return nil
		}
	}

	changes := map[string]any{}

	if gn != (groupName{row.Group, row.Name}) {
		changes["group_name"], changes["name"] = g.Group, g.Name
	}

	if row.StreetID != op.link.streetID || deref(row.HouseNumberID) != deref(op.link.houseNumberID) {
		changes["street_id"], changes["house_number_id"] = op.link.streetID, op.link.houseNumberID
	}

	if row.Latitude != *g.Latitude || row.Longitude != *g.Longitude {
		changes["latitude"], changes["longitude"] = *g.Latitude, *g.Longitude
	}

	if deref(row.ExternalID) != deref(g.ExternalID) {
		changes["external_id"] = g.ExternalID
	}

	if len(changes) == 0 {
		gs.stats.GeoObjects.Unchanged++
		return nil
	}

	if err := gs.tx.Model(&GeoObject{}).Where("id = ?", row.ID).Updates(changes).Error; err != nil {
		return err
	}

	delete(gs.ownedByName, groupName{row.Group, row.Name})
	row.Group, row.Name = g.Group, g.Name
	gs.ownedByName[gn] = row
	gs.stats.GeoObjects.Updated++

	return nil
}

func (gs *geoObjectSync) insert(ops []geoObjectOp) error {
	if len(ops) == 0 {
		return nil
	}

	rows := make([]GeoObject, len(ops))

	for i, op := range ops {
		g := op.planned.geo
		rows[i] = GeoObject{
			Name: g.Name, Group: g.Group, StreetID: op.link.streetID, HouseNumberID: op.link.houseNumberID,
			Latitude: *g.Latitude, Longitude: *g.Longitude, ExternalID: g.ExternalID, CollectorID: &gs.collectorID,
		}
	}

	if err := gs.tx.CreateInBatches(&rows, insertBatchSize).Error; err != nil {
		return err
	}

	gs.stats.GeoObjects.Created += len(rows)

	return nil
}
