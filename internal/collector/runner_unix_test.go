//go:build unix

package collector

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nils-witt/address-serve/internal/store"
	"github.com/nils-witt/address-serve/internal/store/storetest"
)

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

	cs := store.NewCollectorStore(storetest.OpenIsolated(t))
	dir := t.TempDir()
	writeScript(t, dir, "test.sh", collectorScript(collectBody))
	writeScript(t, dir, "broken", "exit 1")

	f := runnerFixture{runner: NewRunner(cs, Config{
		Dir: dir, Timeout: timeout, RunHistory: 10, LogFlushInterval: 50 * time.Millisecond,
	}), cs: cs}

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

func TestRunner_LogWhileRunning(t *testing.T) {
	t.Parallel()

	f := newRunnerFixture(t, `echo "step 1" >&2; sleep 30`, time.Minute)
	ctx := t.Context()

	run, err := f.runner.Start(ctx, "test", "tester")
	if err != nil {
		t.Fatal(err)
	}

	// The flusher saves the log every 50ms; give it ample time on a slow
	// machine.
	deadline := time.Now().Add(10 * time.Second)

	for {
		got, err := f.cs.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}

		if got.Status != store.RunRunning {
			t.Fatalf("run = %+v, want it still running", got)
		}

		if got.Log == "step 1\n" {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("log of the running run = %q, want %q", got.Log, "step 1\n")
		}

		time.Sleep(20 * time.Millisecond)
	}

	if err := f.runner.Cancel(run.ID); err != nil {
		t.Fatal(err)
	}

	f.runner.Wait()

	if run, _ = f.cs.GetRun(ctx, run.ID); run.Status != store.RunCancelled || run.Log != "step 1\n" {
		t.Errorf("cancelled run = %+v, want its log kept", run)
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
