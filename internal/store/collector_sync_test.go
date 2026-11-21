package store

import (
	"fmt"
	"hash/fnv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

// syncFixture is a collector with its own city and patch of the globe, so
// parallel tests neither match each other's streets nor find each other's
// house numbers as the nearest.
type syncFixture struct {
	db   *gorm.DB
	cs   *CollectorStore
	id   string
	city string
	lat  float64
	lon  float64
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()

	db := openBenchDB(t)
	tag := uuid.NewString()

	h := fnv.New32a()
	_, _ = h.Write([]byte(tag))
	sum := h.Sum32()

	f := &syncFixture{
		db: db, cs: NewCollectorStore(db), id: "sync-" + tag, city: "City " + tag,
		// Far from the benchmark data around 50°N 7°E.
		lat: -70 + float64(sum%10000)/200, lon: -170 + float64(sum/10000%10000)/30,
	}
	f.addCollector(t, f.id)

	return f
}

func (f *syncFixture) addCollector(t *testing.T, id string) {
	t.Helper()

	if err := f.db.Create(&Collector{ID: id, Path: "/" + id, Status: CollectorOK}).Error; err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		// Deleting the collector releases what it owns; then remove that too.
		f.db.Delete(&Collector{}, "id = ?", id)
		f.db.Delete(&Street{}, "city = ?", f.city)
	})
}

// addr returns a valid address in the fixture's city, offset by meters
// metres east of the fixture's point.
func (f *syncFixture) addr(street string, number int, meters float64) contract.Address {
	return contract.Address{
		Street: street, District: "District", City: f.city, Country: "XX",
		Number: number, Postcode: "11111", Latitude: new(f.lat), Longitude: new(f.lon + meters/50000),
	}
}

func (f *syncFixture) geo(group, name string, meters float64) contract.GeoObject {
	return contract.GeoObject{Group: group + f.id, Name: name, Latitude: new(f.lat), Longitude: new(f.lon + meters/50000)}
}

func (f *syncFixture) ref(street string, number *int) *contract.AddressRef {
	return &contract.AddressRef{Street: street, District: "District", City: f.city, Country: "XX", Number: number}
}

func (f *syncFixture) syncAs(t *testing.T, collectorID string, out contract.Output) (SyncStats, []SyncIssue) {
	t.Helper()

	out.ContractVersion = contract.Version

	stats, issues, err := f.cs.Sync(t.Context(), collectorID, out, SyncOptions{GeoMaxDistanceMeters: 100})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	return stats, issues
}

func (f *syncFixture) sync(t *testing.T, out contract.Output) (SyncStats, []SyncIssue) {
	t.Helper()
	return f.syncAs(t, f.id, out)
}

func (f *syncFixture) street(t *testing.T, name string) Street {
	t.Helper()

	var s Street
	if err := f.db.Take(&s, "name = ? AND city = ?", name, f.city).Error; err != nil {
		t.Fatalf("street %s: %v", name, err)
	}

	return s
}

func (f *syncFixture) houseNumbers(t *testing.T, streetName string) []HouseNumber {
	t.Helper()

	var rows []HouseNumber

	err := f.db.Joins("JOIN streets s ON s.id = house_numbers.street_id").
		Where("s.name = ? AND s.city = ?", streetName, f.city).Order("number").Find(&rows).Error
	if err != nil {
		t.Fatal(err)
	}

	return rows
}

func (f *syncFixture) geoObjects(t *testing.T) []GeoObject {
	t.Helper()

	var rows []GeoObject
	if err := f.db.Where("group_name LIKE ?", "%"+f.id).Order("name").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}

	return rows
}

func wantStats(t *testing.T, what string, got, want EntityStats) {
	t.Helper()

	if got != want {
		t.Errorf("%s stats = %+v, want %+v", what, got, want)
	}
}

func wantIssue(t *testing.T, issues []SyncIssue, kind IssueKind, reason string) SyncIssue {
	t.Helper()

	for _, i := range issues {
		if i.Kind == kind && strings.Contains(i.Reason, reason) {
			return i
		}
	}

	t.Fatalf("no %s issue mentioning %q in %+v", kind, reason, issues)

	return SyncIssue{}
}

func numbers(rows []HouseNumber) []int {
	out := make([]int, len(rows))
	for i, h := range rows {
		out[i] = h.Number
	}

	return out
}

func TestSync_CreatesUpdatesAndDeletes(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	out := contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0), f.addr("A", 2, 10), f.addr("B", 1, 50)}}

	stats, issues := f.sync(t, out)
	wantStats(t, "streets", stats.Streets, EntityStats{Created: 2})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Created: 3})

	if len(issues) != 0 {
		t.Errorf("issues = %+v", issues)
	}

	a := f.street(t, "A")
	if a.CollectorID == nil || *a.CollectorID != f.id {
		t.Errorf("street A owned by %v, want %s", a.CollectorID, f.id)
	}

	stats, _ = f.sync(t, out)
	wantStats(t, "streets", stats.Streets, EntityStats{Unchanged: 2})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Unchanged: 3})

	moved := f.addr("A", 1, 5)
	moved.Postcode = "22222"
	stats, _ = f.sync(t, contract.Output{Addresses: []contract.Address{moved}})
	wantStats(t, "streets", stats.Streets, EntityStats{Updated: 1, Deleted: 1})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Updated: 1, Deleted: 2})

	hns := f.houseNumbers(t, "A")
	if len(hns) != 1 || hns[0].Postcode != "22222" || hns[0].Longitude != *moved.Longitude {
		t.Errorf("house numbers of A = %+v", hns)
	}

	if got := f.street(t, "A"); got.ID != a.ID || got.Longitude != *moved.Longitude {
		t.Errorf("street A = %+v, want the same row at the moved point", got)
	}

	stats, _ = f.sync(t, contract.Output{})
	wantStats(t, "streets", stats.Streets, EntityStats{Deleted: 1})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Deleted: 1})
}

func TestSync_StreetAtMedoid(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	f.sync(t, contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0), f.addr("A", 2, 100), f.addr("A", 3, 40)}})

	if got, want := f.street(t, "A").Longitude, f.lon+40.0/50000; got != want {
		t.Errorf("street longitude = %v, want %v (house number 3's)", got, want)
	}
}

func TestSync_ForeignStreetsAreConflicts(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)

	manual, err := NewStreetStore(f.db).Create(t.Context(), Street{
		Name: "Manual", District: "District", City: f.city, Country: "XX", Latitude: f.lat, Longitude: f.lon,
	})
	if err != nil {
		t.Fatal(err)
	}

	other := "other-" + f.id
	f.addCollector(t, other)
	f.syncAs(t, other, contract.Output{Addresses: []contract.Address{f.addr("Theirs", 1, 0)}})

	stats, issues := f.sync(t, contract.Output{Addresses: []contract.Address{
		f.addr("Manual", 1, 0), f.addr("Manual", 2, 0), f.addr("Theirs", 1, 0), f.addr("Mine", 1, 0),
	}})
	wantStats(t, "streets", stats.Streets, EntityStats{Created: 1, Conflicts: 2})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Created: 1, Skipped: 3})

	if i := wantIssue(t, issues, IssueConflict, "manually created"); i.RowID == nil || *i.RowID != manual.ID {
		t.Errorf("conflict = %+v, want it to name the manual street", i)
	}

	wantIssue(t, issues, IssueConflict, `collector "`+other+`"`)

	if hns := f.houseNumbers(t, "Manual"); len(hns) != 0 {
		t.Errorf("manual street got house numbers %v", numbers(hns))
	}
}

func TestSync_AdoptsManualHouseNumbers(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	f.sync(t, contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0)}})

	manual, err := NewHouseNumberStore(f.db).Create(t.Context(), HouseNumber{
		StreetID: f.street(t, "A").ID, Number: 2, Postcode: "1", Latitude: f.lat, Longitude: f.lon,
	})
	if err != nil {
		t.Fatal(err)
	}

	stats, issues := f.sync(t, contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0), f.addr("A", 2, 0)}})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Unchanged: 1, Adopted: 1})

	if i := wantIssue(t, issues, IssueAdopted, "manually created"); *i.RowID != manual.ID {
		t.Errorf("adopted %v, want %v", i.RowID, manual.ID)
	}

	hns := f.houseNumbers(t, "A")
	if len(hns) != 2 || hns[1].ID != manual.ID || hns[1].CollectorID == nil || hns[1].Postcode != "11111" {
		t.Errorf("house numbers = %+v, want the manual one adopted and updated", hns)
	}

	stats, _ = f.sync(t, contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0)}})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Unchanged: 1, Deleted: 1})
}

func TestSync_RenamesByExternalID(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	// External IDs are unique across the database, so each test needs its own.
	withIDs := func(a contract.Address, street, number string) contract.Address {
		a.StreetExternalID, a.HouseNumberExternalID = new(street+f.id), new(number+f.id)
		return a
	}

	f.sync(t, contract.Output{Addresses: []contract.Address{
		withIDs(f.addr("Old", 5, 0), "s1", "h1"), withIDs(f.addr("Other", 1, 0), "s2", "h2"),
	}})
	old, oldNumber := f.street(t, "Old"), f.houseNumbers(t, "Old")[0]

	stats, _ := f.sync(t, contract.Output{Addresses: []contract.Address{
		withIDs(f.addr("New", 6, 0), "s1", "h1"), withIDs(f.addr("Other", 1, 0), "s2", "h2"),
	}})
	wantStats(t, "streets", stats.Streets, EntityStats{Updated: 1, Unchanged: 1})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Updated: 1, Unchanged: 1})

	renamed := f.street(t, "New")
	if renamed.ID != old.ID {
		t.Errorf("renamed street has ID %v, want %v", renamed.ID, old.ID)
	}

	if hns := f.houseNumbers(t, "New"); len(hns) != 1 || hns[0].ID != oldNumber.ID || hns[0].Number != 6 {
		t.Errorf("house numbers of New = %+v, want %v renumbered to 6", hns, oldNumber.ID)
	}

	// Renaming onto the name of another street of the collector, even one it
	// is about to delete, is a conflict. The street keeps its old name and
	// house numbers rather than being deleted as stale.
	stats, issues := f.sync(t, contract.Output{Addresses: []contract.Address{withIDs(f.addr("New", 1, 0), "s2", "h2")}})
	wantStats(t, "streets", stats.Streets, EntityStats{Conflicts: 1, Deleted: 1})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Skipped: 1, Deleted: 1})
	wantIssue(t, issues, IssueConflict, "another street of this collector has this name")

	if hns := f.houseNumbers(t, "Other"); len(hns) != 1 {
		t.Errorf("house numbers of the kept street = %v, want its one", numbers(hns))
	}
}

func TestSync_BackfillsExternalIDs(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	f.sync(t, contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0)}})
	before := f.street(t, "A")

	a := f.addr("A", 1, 0)
	a.StreetExternalID, a.HouseNumberExternalID = new("s"+f.id), new("h"+f.id)
	f.sync(t, contract.Output{Addresses: []contract.Address{a}})

	after := f.street(t, "A")
	if after.ID != before.ID || deref(after.ExternalID) != "s"+f.id {
		t.Errorf("street = %+v, want %v with the new external ID", after, before.ID)
	}

	if hns := f.houseNumbers(t, "A"); deref(hns[0].ExternalID) != "h"+f.id {
		t.Errorf("house number external ID = %v", hns[0].ExternalID)
	}
}

func TestSync_InvalidEntryDeletesItsRow(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	f.sync(t, contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0), f.addr("A", 2, 0)}})

	broken := f.addr("A", 2, 0)
	broken.Postcode = ""

	stats, issues := f.sync(t, contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0), broken}})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Unchanged: 1, Deleted: 1, Skipped: 1})

	if i := wantIssue(t, issues, IssueSkipped, "postcode"); i.Index == nil || *i.Index != 1 {
		t.Errorf("issue = %+v, want it at index 1", i)
	}
}

func TestSync_ReleasesStaleRowsOthersDependOn(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	f.sync(t, contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0), f.addr("B", 1, 0), f.addr("C", 1, 0)}})

	a, b := f.street(t, "A"), f.street(t, "B")
	hnA := f.houseNumbers(t, "A")[0]

	// A manual geo object on A's house number, and a manual house number on B.
	_, err := NewGeoObjectStore(f.db).Create(t.Context(), GeoObject{
		Name: "manual", Group: "g" + f.id, StreetID: a.ID, HouseNumberID: &hnA.ID, Latitude: f.lat, Longitude: f.lon,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewHouseNumberStore(f.db).Create(t.Context(), HouseNumber{
		StreetID: b.ID, Number: 9, Postcode: "1", Latitude: f.lat, Longitude: f.lon,
	}); err != nil {
		t.Fatal(err)
	}

	stats, issues := f.sync(t, contract.Output{})
	wantStats(t, "streets", stats.Streets, EntityStats{Deleted: 1, Released: 2})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Deleted: 2, Released: 1})
	wantIssue(t, issues, IssueReleased, "geo objects of others")
	wantIssue(t, issues, IssueReleased, "other rows are on it")

	for _, name := range []string{"A", "B"} {
		if st := f.street(t, name); st.CollectorID != nil {
			t.Errorf("street %s still owned by %v", name, *st.CollectorID)
		}
	}

	if hns := f.houseNumbers(t, "A"); len(hns) != 1 || hns[0].CollectorID != nil {
		t.Errorf("house numbers of A = %+v, want the released one", hns)
	}

	if geo := f.geoObjects(t); len(geo) != 1 || geo[0].HouseNumberID == nil || *geo[0].HouseNumberID != hnA.ID {
		t.Errorf("manual geo object = %+v, want it untouched", geo)
	}
}

func TestSync_GeoObjects(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	addresses := []contract.Address{f.addr("A", 1, 0), f.addr("A", 2, 1000)}

	byRef := f.geo("hydrant", "by ref", 1000)
	byRef.Address = f.ref("A", new(2))
	nearest := f.geo("hydrant", "nearest", 20) // 20 m from A 1
	byExt := f.geo("hydrant", "by ext", 5)
	byExt.ExternalID = new("ext-" + f.id)
	far := f.geo("hydrant", "far", 3000)
	unknownStreet := f.geo("hydrant", "unknown street", 10)
	unknownStreet.Address = f.ref("Nowhere", nil)

	stats, issues := f.sync(t, contract.Output{
		Addresses: addresses, GeoObjects: []contract.GeoObject{byRef, nearest, byExt, far, unknownStreet},
	})
	wantStats(t, "geo objects", stats.GeoObjects, EntityStats{Created: 4, Skipped: 1})
	wantIssue(t, issues, IssueSkipped, "no house number within 100 m")

	hns := f.houseNumbers(t, "A")
	placed := map[string]*uuid.UUID{}

	for _, g := range f.geoObjects(t) {
		placed[g.Name] = g.HouseNumberID
	}

	for name, want := range map[string]uuid.UUID{"by ref": hns[1].ID, "nearest": hns[0].ID, "unknown street": hns[0].ID} {
		if got := placed[name]; got == nil || *got != want {
			t.Errorf("%s placed at %v, want %v", name, got, want)
		}
	}

	// Renaming by external ID, dropping one, and keeping the rest.
	byExt.Name = "renamed"
	stats, _ = f.sync(t, contract.Output{Addresses: addresses, GeoObjects: []contract.GeoObject{byRef, nearest, byExt}})
	wantStats(t, "geo objects", stats.GeoObjects, EntityStats{Updated: 1, Unchanged: 2, Deleted: 1})

	if _, ok := placed["by ext"]; !ok {
		t.Fatal("by ext was not created")
	}
}

func TestSync_GeoObjectConflicts(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	f.sync(t, contract.Output{Addresses: []contract.Address{f.addr("A", 1, 0)}})

	manual, err := NewGeoObjectStore(f.db).Create(t.Context(), GeoObject{
		Name: "taken", Group: "hydrant" + f.id, StreetID: f.street(t, "A").ID, Latitude: f.lat, Longitude: f.lon,
		ExternalID: new("manual-" + f.id),
	})
	if err != nil {
		t.Fatal(err)
	}

	sameName := f.geo("hydrant", "taken", 0)
	sameExt := f.geo("hydrant", "other", 0)
	sameExt.ExternalID = manual.ExternalID

	stats, issues := f.sync(t, contract.Output{
		Addresses:  []contract.Address{f.addr("A", 1, 0)},
		GeoObjects: []contract.GeoObject{sameName, sameExt},
	})
	wantStats(t, "geo objects", stats.GeoObjects, EntityStats{Conflicts: 2})
	wantIssue(t, issues, IssueConflict, "the name is used in this group")
	wantIssue(t, issues, IssueConflict, "externalId is used")

	if got := f.geoObjects(t); len(got) != 1 || got[0].CollectorID != nil {
		t.Errorf("geo objects = %+v, want only the untouched manual one", got)
	}
}

func TestSync_FailureAppliesNothing(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)

	// Rows can't reference an unknown collector, so the insert fails.
	_, _, err := f.cs.Sync(t.Context(), "unknown-"+f.id, contract.Output{
		ContractVersion: contract.Version, Addresses: []contract.Address{f.addr("A", 1, 0)},
	}, SyncOptions{})
	if err == nil {
		t.Fatal("Sync succeeded for an unknown collector")
	}

	var count int64
	if err := f.db.Model(&Street{}).Where("city = ?", f.city).Count(&count).Error; err != nil || count != 0 {
		t.Errorf("streets after failed sync = %d (%v), want 0", count, err)
	}
}

func TestSync_ConflictingEntryKeepsItsRow(t *testing.T) {
	t.Parallel()

	f := newSyncFixture(t)
	a := f.addr("A", 1, 0)
	a.HouseNumberExternalID = new("h1-" + f.id)
	g := f.geo("hydrant", "n", 0)
	g.ExternalID = new("e1-" + f.id)

	f.sync(t, contract.Output{Addresses: []contract.Address{a}, GeoObjects: []contract.GeoObject{g}})

	// Same number and name, but the source now claims other identities.
	a.HouseNumberExternalID = new("h2-" + f.id)
	g.ExternalID = new("e2-" + f.id)

	stats, issues := f.sync(t, contract.Output{Addresses: []contract.Address{a}, GeoObjects: []contract.GeoObject{g}})
	wantStats(t, "house numbers", stats.HouseNumbers, EntityStats{Conflicts: 1})
	wantStats(t, "geo objects", stats.GeoObjects, EntityStats{Conflicts: 1})
	wantIssue(t, issues, IssueConflict, "different externalId: h1-")

	if hns := f.houseNumbers(t, "A"); len(hns) != 1 || deref(hns[0].ExternalID) != "h1-"+f.id {
		t.Errorf("house numbers = %+v, want the original kept", hns)
	}

	if geo := f.geoObjects(t); len(geo) != 1 || deref(geo[0].ExternalID) != "e1-"+f.id {
		t.Errorf("geo objects = %+v, want the original kept", geo)
	}
}

// BenchmarkSync_Unchanged measures a resync of 20000 addresses on 1000 streets
// that are all stored already, the common case for a collector run.
func BenchmarkSync_Unchanged(b *testing.B) {
	db := openBenchDB(b)
	cs := NewCollectorStore(db)
	id := "bench-" + uuid.NewString()

	if err := db.Create(&Collector{ID: id, Path: "/bench", Status: CollectorOK}).Error; err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() {
		db.Exec("DELETE FROM streets WHERE collector_id = ?", id)
		db.Delete(&Collector{}, "id = ?", id)
	})

	out := contract.Output{ContractVersion: contract.Version}

	for s := range 1000 {
		for n := range 20 {
			out.Addresses = append(out.Addresses, contract.Address{
				Street: fmt.Sprintf("Bench %d", s), District: "D", City: id, Country: "XX",
				Number: n + 1, Postcode: "1", Latitude: new(-60 + float64(s)/1000), Longitude: new(100 + float64(n)/1000),
			})
		}
	}

	if _, _, err := cs.Sync(b.Context(), id, out, SyncOptions{}); err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		stats, _, err := cs.Sync(b.Context(), id, out, SyncOptions{})
		if err != nil || stats.HouseNumbers.Unchanged != 20000 {
			b.Fatalf("Sync = %+v, %v", stats.HouseNumbers, err)
		}
	}
}
