//go:build unix

package api

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/nils-witt/address-serve/internal/collector"
	"github.com/nils-witt/address-serve/internal/store"
	"github.com/nils-witt/address-serve/internal/store/storetest"
)

// demoCollector describes a required street setting and a secret token,
// fails unless the token arrives, and returns one address on the street.
const demoCollector = `#!/bin/sh
case "$1" in
describe) echo '{"contractVersion":1,"name":"Demo","version":"1","settings":[
  {"key":"street","label":"Street","type":"string","required":true},
  {"key":"token","label":"Token","type":"secret"}]}';;
collect)
  [ "$COLLECTOR_SETTING_TOKEN" = "s3cret" ] || { echo "wrong token" >&2; exit 1; }
  echo '{"contractVersion":1,"addresses":[{"street":"'"$COLLECTOR_SETTING_STREET"'","district":"D","city":"C","country":"X","number":1,"postcode":"1","latitude":1,"longitude":2}]}';;
esac
`

const slowCollector = `#!/bin/sh
case "$1" in
describe) echo '{"contractVersion":1,"name":"Slow","version":"1"}';;
collect) sleep 30;;
esac
`

type collectorAPIFixture struct {
	apiClient

	runner   *collector.Runner
	adminJWT string
	userJWT  string
}

func newCollectorAPIFixture(t *testing.T) collectorAPIFixture {
	t.Helper()

	db := storetest.OpenIsolated(t)

	dir := t.TempDir()
	for name, script := range map[string]string{"demo.sh": demoCollector, "slow": slowCollector} {
		//nolint:gosec // a collector has to be executable
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	provider := newFakeProvider(t, key)

	oidc, err := NewOIDCVerifier(t.Context(), OIDCConfig{IssuerURL: provider.URL, Audience: testAudience})
	if err != nil {
		t.Fatal(err)
	}

	cs := store.NewCollectorStore(db)
	runner := collector.NewRunner(cs, collector.Config{Dir: dir, Timeout: time.Minute, RunHistory: 10})
	t.Cleanup(runner.Wait)

	handler := NewHandler(Config{
		OIDC:       oidc,
		Collectors: CollectorConfig{Runner: runner, Store: cs, Admin: AdminClaim{Claim: "realm_access.roles", Value: adminRole}},
	}, store.NewStreetStore(db), nil, nil, nil, store.NewAPITokenStore(db))

	sign := func(roles ...any) string {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": provider.URL, "aud": testAudience, subClaim: testSubject, "name": "Ada",
			"exp": time.Now().Add(time.Hour).Unix(), "realm_access": map[string]any{"roles": roles},
		})
		token.Header["kid"] = testKID

		s, err := token.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}

		return s
	}

	return collectorAPIFixture{
		t: t, handler: handler,
		runner: runner, adminJWT: sign(adminRole), userJWT: sign("user"),
	}
}

func TestCollectorAPI_ConfigureAndRun(t *testing.T) {
	t.Parallel()

	f := newCollectorAPIFixture(t)
	admin := f.adminJWT

	f.expect(http.MethodGet, "/api/collectors", f.userJWT, "", http.StatusForbidden, nil)

	var list []collectorView
	f.expect(http.MethodPost, "/api/collectors/reload", admin, "", http.StatusOK, &list)

	if len(list) != 2 || list[0].ID != "demo" || list[0].Status != store.CollectorOK || len(list[0].SettingsSchema) != 2 {
		t.Fatalf("reload = %+v", list)
	}

	f.expect(http.MethodPost, "/api/collectors/demo/runs", admin, "", http.StatusBadRequest, nil)
	f.configureDemo()

	run := f.runDemo()

	var (
		runs []store.CollectorRun
		view collectorView
	)

	f.expect(http.MethodGet, "/api/collectors/demo/runs?limit=5", admin, "", http.StatusOK, &runs)
	f.expect(http.MethodGet, "/api/collectors/demo", admin, "", http.StatusOK, &view)

	if len(runs) != 1 || view.LastRun == nil || view.LastRun.ID != run.ID {
		t.Errorf("runs = %+v, last run = %+v", runs, view.LastRun)
	}

	var streets []store.Street
	f.expect(http.MethodGet, "/api/streets?name=Main", admin, "", http.StatusOK, &streets)

	if len(streets) != 1 || streets[0].CollectorID == nil || *streets[0].CollectorID != "demo" {
		t.Fatalf("streets = %+v, want Main owned by demo", streets)
	}

	// Only the collector's runs may change its rows.
	if body := f.expect(http.MethodDelete, "/api/streets/"+streets[0].ID.String(), f.userJWT, "", http.StatusConflict, nil); !strings.Contains(body, `collector \"demo\"`) {
		t.Errorf("delete owned street = %s, want it to name the collector", body)
	}

	f.expect(http.MethodDelete, "/api/collectors/demo", admin, "", http.StatusConflict, nil)
	f.expect(http.MethodGet, "/api/collectors/demo/runs?limit=0", admin, "", http.StatusBadRequest, nil)
	f.expect(http.MethodGet, "/api/collectors/nope", admin, "", http.StatusNotFound, nil)
	f.expect(http.MethodGet, "/api/collectors/nope/runs", admin, "", http.StatusNotFound, nil)
}

// configureDemo sets the demo collector's settings, checking that its secret
// is never returned and is kept when left out.
func (f collectorAPIFixture) configureDemo() {
	f.t.Helper()

	var view collectorView

	body := f.expect(http.MethodPut, "/api/collectors/demo/settings", f.adminJWT, `{"street":"Main","token":"s3cret"}`, http.StatusOK, &view)
	if strings.Contains(body, "s3cret") || len(view.SecretsSet) != 1 || string(view.Settings["street"]) != `"Main"` {
		f.t.Fatalf("settings response = %s, want the secret hidden", body)
	}

	f.expect(http.MethodPut, "/api/collectors/demo/settings", f.adminJWT, `{"street":"Main"}`, http.StatusOK, &view)

	if len(view.SecretsSet) != 1 {
		f.t.Fatalf("secretsSet = %v, want the token kept", view.SecretsSet)
	}

	f.expect(http.MethodPut, "/api/collectors/demo/settings", f.adminJWT, `{"nope":1}`, http.StatusBadRequest, nil)
}

// runDemo runs the demo collector to its end and returns the finished run.
func (f collectorAPIFixture) runDemo() store.CollectorRun {
	f.t.Helper()

	var run store.CollectorRun

	f.expect(http.MethodPost, "/api/collectors/demo/runs", f.adminJWT, "", http.StatusAccepted, &run)
	f.runner.Wait()
	f.expect(http.MethodGet, "/api/collector-runs/"+run.ID.String(), f.adminJWT, "", http.StatusOK, &run)

	if run.Status != store.RunSucceeded || run.TriggeredBy != "Ada" || run.Stats.Streets.Created != 1 {
		f.t.Fatalf("run = %+v, want succeeded", run)
	}

	return run
}

func TestCollectorAPI_Cancel(t *testing.T) {
	t.Parallel()

	f := newCollectorAPIFixture(t)
	admin := f.adminJWT

	f.expect(http.MethodPost, "/api/collectors/reload", admin, "", http.StatusOK, nil)

	var run store.CollectorRun
	f.expect(http.MethodPost, "/api/collectors/slow/runs", admin, "", http.StatusAccepted, &run)
	f.expect(http.MethodPost, "/api/collectors/slow/runs", admin, "", http.StatusConflict, nil)
	f.expect(http.MethodPost, "/api/collector-runs/"+run.ID.String()+"/cancel", admin, "", http.StatusAccepted, nil)
	f.runner.Wait()

	f.expect(http.MethodGet, "/api/collector-runs/"+run.ID.String(), admin, "", http.StatusOK, &run)

	if run.Status != store.RunCancelled {
		t.Errorf("run = %+v, want cancelled", run)
	}

	f.expect(http.MethodPost, "/api/collector-runs/"+run.ID.String()+"/cancel", admin, "", http.StatusConflict, nil)
	f.expect(http.MethodPost, "/api/collector-runs/00000000-0000-0000-0000-000000000000/cancel", admin, "", http.StatusNotFound, nil)
	f.expect(http.MethodGet, "/api/collector-runs/not-a-uuid", admin, "", http.StatusBadRequest, nil)
}
