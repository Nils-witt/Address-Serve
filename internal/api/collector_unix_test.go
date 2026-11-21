//go:build unix

package api

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
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
collect) echo "starting" >&2; sleep 30;;
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
	runner := collector.NewRunner(cs, collector.Config{
		Dir: dir, Timeout: time.Minute, RunHistory: 10, LogFlushInterval: 50 * time.Millisecond,
	})
	t.Cleanup(runner.Wait)

	handler := NewHandler(Config{
		OIDC:       oidc,
		Collectors: CollectorConfig{Runner: runner, Store: cs, AdminGroup: adminGroup},
	}, store.NewStreetStore(db), nil, nil, nil, store.NewAPITokenStore(db))

	sign := func(groups ...any) string {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": provider.URL, "aud": testAudience, subClaim: testSubject, "name": "Ada",
			"exp": time.Now().Add(time.Hour).Unix(), groupsClaim: groups,
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
		runner: runner, adminJWT: sign("users", adminGroup), userJWT: sign("users"),
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

	// The log so far shows while the run runs.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		var live store.CollectorRun
		f.expect(http.MethodGet, "/api/collector-runs/"+run.ID.String(), admin, "", http.StatusOK, &live)

		if live.Status == store.RunRunning && live.Log == "starting\n" {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("running run = %+v, want status running and log %q", live, "starting\n")
		}
	}

	f.expect(http.MethodPost, "/api/collector-runs/"+run.ID.String()+"/cancel", admin, "", http.StatusAccepted, nil)
	f.runner.Wait()

	f.expect(http.MethodGet, "/api/collector-runs/"+run.ID.String(), admin, "", http.StatusOK, &run)

	if run.Status != store.RunCancelled || run.Log != "starting\n" {
		t.Errorf("run = %+v, want cancelled with its log", run)
	}

	f.expect(http.MethodPost, "/api/collector-runs/"+run.ID.String()+"/cancel", admin, "", http.StatusConflict, nil)
	f.expect(http.MethodPost, "/api/collector-runs/00000000-0000-0000-0000-000000000000/cancel", admin, "", http.StatusNotFound, nil)
	f.expect(http.MethodGet, "/api/collector-runs/not-a-uuid", admin, "", http.StatusBadRequest, nil)
}

func TestCollectorAPI_Events(t *testing.T) {
	t.Parallel()

	f := newCollectorAPIFixture(t)
	f.configureDemoAfterReload()

	server := httptest.NewServer(f.handler)
	t.Cleanup(server.Close)

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/collector-events"

	if _, status, err := dialEvents(t, url, eventsProtocol, tokenProtocolPrefix+f.userJWT); err == nil || status != http.StatusForbidden {
		t.Fatalf("dial as non-admin: err %v, status %d, want 403", err, status)
	}

	if _, status, err := dialEvents(t, url, eventsProtocol); err == nil || status != http.StatusUnauthorized {
		t.Fatalf("dial without a token: err %v, status %d, want 401", err, status)
	}

	conn, _, err := dialEvents(t, url, eventsProtocol, tokenProtocolPrefix+f.adminJWT)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = conn.CloseNow() }()

	if conn.Subprotocol() != eventsProtocol {
		t.Fatalf("subprotocol = %q, want %q", conn.Subprotocol(), eventsProtocol)
	}

	var started store.CollectorRun
	f.expect(http.MethodPost, "/api/collectors/demo/runs", f.adminJWT, "", http.StatusAccepted, &started)

	statuses := readRunStatuses(t, conn, started.ID.String())
	if statuses[0] != store.RunRunning || statuses[len(statuses)-1] != store.RunSucceeded {
		t.Errorf("statuses = %v, want running first and succeeded last", statuses)
	}
}

// dialEvents opens the events WebSocket offering protocols, and returns the
// handshake's HTTP status, if there was a response.
func dialEvents(t *testing.T, url string, protocols ...string) (*websocket.Conn, int, error) {
	t.Helper()

	conn, res, err := websocket.Dial(t.Context(), url, &websocket.DialOptions{Subprotocols: protocols})
	if res == nil {
		return conn, 0, err
	}

	if res.Body != nil { // nil after a successful handshake
		_ = res.Body.Close()
	}

	return conn, res.StatusCode, err
}

// readRunStatuses reads run events for runID until the run ends, and
// returns the statuses they carried.
func readRunStatuses(t *testing.T, conn *websocket.Conn, runID string) []store.RunStatus {
	t.Helper()

	var statuses []store.RunStatus

	for {
		var event collectorEvent
		if err := wsjson.Read(t.Context(), conn, &event); err != nil {
			t.Fatalf("read event after %v: %v", statuses, err)
		}

		if event.Type != "run" || event.Run.ID.String() != runID || event.Run.Log != "" {
			t.Fatalf("event = %+v, want a run event for %s without its log", event, runID)
		}

		statuses = append(statuses, event.Run.Status)
		if event.Run.Status != store.RunRunning {
			return statuses
		}
	}
}

// configureDemoAfterReload discovers the collectors and configures demo.
func (f collectorAPIFixture) configureDemoAfterReload() {
	f.t.Helper()
	f.expect(http.MethodPost, "/api/collectors/reload", f.adminJWT, "", http.StatusOK, nil)
	f.configureDemo()
}
