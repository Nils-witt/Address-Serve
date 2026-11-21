//go:build unix

package collector

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/store"
)

// openIsolatedStore returns a CollectorStore whose tables live in a fresh
// schema, dropped when the test ends: Reload marks every collector it
// doesn't find as missing, which would disturb other tests' collectors.
func openIsolatedStore(t *testing.T) *store.CollectorStore {
	t.Helper()

	dsn := os.Getenv("ADDRESS_SERV_TEST_DSN")
	if dsn == "" {
		t.Skip("ADDRESS_SERV_TEST_DSN not set")
	}

	admin, err := store.Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close(admin) })

	// The extensions live in public, where the schema's search_path finds
	// them; Migrate creates them there if no other test did yet.
	if err := store.Migrate(t.Context(), admin); err != nil {
		t.Fatal(err)
	}

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}

	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()

	db, err := store.Open(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = store.Close(db) })

	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}

	return store.NewCollectorStore(db)
}

// collectorScript is a collector that describes itself with one required
// setting, street, and returns one address on that street. Its stdout and
// stderr for collect are replaced when collectBody is set.
func collectorScript(collectBody string) string {
	if collectBody == "" {
		collectBody = `echo "collecting $COLLECTOR_SETTING_STREET" >&2
echo '{"contractVersion":1,"addresses":[{"street":"'"$COLLECTOR_SETTING_STREET"'","district":"D","city":"C","country":"X","number":1,"postcode":"1","latitude":1,"longitude":2}]}'`
	}

	return `case "$1" in
describe) echo '{"contractVersion":1,"name":"Test","version":"1","settings":[{"key":"street","label":"Street","type":"string","required":true}]}';;
collect) ` + collectBody + `;;
esac
`
}

type runnerFixture struct {
	runner *Runner
	cs     *store.CollectorStore
}

func newRunnerFixture(t *testing.T, collectBody string, timeout time.Duration) runnerFixture {
	t.Helper()

	cs := openIsolatedStore(t)
	dir := t.TempDir()
	writeScript(t, dir, "test.sh", collectorScript(collectBody))
	writeScript(t, dir, "broken", "exit 1")

	f := runnerFixture{runner: NewRunner(cs, Config{Dir: dir, Timeout: timeout, RunHistory: 10}), cs: cs}

	collectors, err := f.runner.Reload(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if len(collectors) != 2 || collectors[1].ID != "test" || collectors[1].Status != store.CollectorOK {
		t.Fatalf("Reload = %+v", collectors)
	}

	if _, err := cs.UpdateSettings(t.Context(), "test", store.SettingsInput{"street": json.RawMessage(`"Main"`)}); err != nil {
		t.Fatal(err)
	}

	return f
}

// run starts the test collector, waits for it, and returns the finished run.
func (f runnerFixture) run(t *testing.T) store.CollectorRun {
	t.Helper()

	run, err := f.runner.Start(t.Context(), "test", "tester")
	if err != nil {
		t.Fatal(err)
	}

	f.runner.Wait()

	run, err = f.cs.GetRun(t.Context(), run.ID)
	if err != nil {
		t.Fatal(err)
	}

	return run
}

func TestRunner_Succeeds(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t, "", time.Minute)
	run := f.run(t)

	if run.Status != store.RunSucceeded || run.Error != nil || run.ExitCode == nil || *run.ExitCode != 0 {
		t.Fatalf("run = %+v, want succeeded", run)
	}

	if run.Stats == nil || run.Stats.Streets.Created != 1 || run.Stats.HouseNumbers.Created != 1 {
		t.Errorf("stats = %+v, want one street and house number created", run.Stats)
	}

	if run.Log != "collecting Main\n" || run.TriggeredBy != "tester" || run.FinishedAt == nil {
		t.Errorf("run = %+v, want the log, trigger and finish time", run)
	}
}

func TestRunner_StartRefusals(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t, "sleep 30", time.Minute)
	ctx := t.Context()

	if _, err := f.runner.Start(ctx, "broken", "tester"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("invalid collector: err = %v, want ErrUnavailable", err)
	}

	if _, err := f.runner.Start(ctx, "absent", "tester"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("unknown collector: err = %v, want ErrNotFound", err)
	}

	if _, err := f.cs.UpdateSettings(ctx, "test", store.SettingsInput{}); err != nil {
		t.Fatal(err)
	}

	var missing *MissingSettingsError
	if _, err := f.runner.Start(ctx, "test", "tester"); !errors.As(err, &missing) || missing.Keys[0] != "street" {
		t.Errorf("missing setting: err = %v, want a MissingSettingsError for street", err)
	}

	if _, err := f.cs.UpdateSettings(ctx, "test", store.SettingsInput{"street": json.RawMessage(`"Main"`)}); err != nil {
		t.Fatal(err)
	}

	run, err := f.runner.Start(ctx, "test", "tester")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.runner.Start(ctx, "test", "tester"); !errors.Is(err, store.ErrRunActive) {
		t.Errorf("second start: err = %v, want ErrRunActive", err)
	}

	if err := f.runner.Cancel(run.ID); err != nil {
		t.Fatal(err)
	}

	f.runner.Wait()

	if err := f.runner.Cancel(run.ID); !errors.Is(err, ErrRunNotActive) {
		t.Errorf("cancelling a finished run: err = %v, want ErrRunNotActive", err)
	}

	if run, _ = f.cs.GetRun(ctx, run.ID); run.Status != store.RunCancelled || run.Stats != nil {
		t.Errorf("cancelled run = %+v", run)
	}
}

func TestRunner_Failures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		collect   string
		timeout   time.Duration
		wantError string
		wantExit  *int
	}{
		{name: "non-zero exit", collect: "echo failed >&2; exit 3", wantError: "exited with status 3", wantExit: new(3)},
		{name: "invalid output", collect: "echo nope", wantError: "parse collect output", wantExit: new(0)},
		{name: "wrong contract version", collect: `echo '{"contractVersion":2}'`, wantError: "unsupported contractVersion", wantExit: new(0)},
		{name: "timeout", collect: "sleep 30", timeout: 300 * time.Millisecond, wantError: "timed out after 300ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			timeout := tt.timeout
			if timeout == 0 {
				timeout = time.Minute
			}

			run := newRunnerFixture(t, tt.collect, timeout).run(t)

			if run.Status != store.RunFailed || run.Error == nil || !strings.Contains(*run.Error, tt.wantError) {
				t.Errorf("run = %+v, want failed with %q", run, tt.wantError)
			}

			if (run.ExitCode == nil) != (tt.wantExit == nil) || (run.ExitCode != nil && *run.ExitCode != *tt.wantExit) {
				t.Errorf("exit code = %v, want %v", run.ExitCode, tt.wantExit)
			}
		})
	}
}
