package collector

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/collector/contract"
	"github.com/nils-witt/address-serve/internal/store"
)

// DescribeTimeout bounds `<exe> describe`.
const DescribeTimeout = 10 * time.Second

var (
	// ErrUnavailable is returned when starting a collector that is missing
	// or invalid.
	ErrUnavailable = errors.New("the collector is missing or invalid and can't run")

	// ErrRunNotActive is returned when cancelling a run that isn't running
	// in this process.
	ErrRunNotActive = errors.New("the run is not running")

	// errCancelled is the cause of a run's context when it was cancelled.
	errCancelled = errors.New("cancelled")
)

// MissingSettingsError is returned when starting a collector whose required
// settings have no value.
type MissingSettingsError struct {
	Keys []string
}

func (e *MissingSettingsError) Error() string {
	return "required settings have no value: " + strings.Join(e.Keys, ", ")
}

// Config configures a Runner.
type Config struct {
	// Dir is the collectors folder.
	Dir string
	// Timeout bounds a run's collector process.
	Timeout time.Duration
	// GeoMaxDistanceMeters bounds the nearest-house-number fallback for geo
	// objects.
	GeoMaxDistanceMeters float64
	// RunHistory is how many runs to keep per collector.
	RunHistory int
	// LogFlushInterval is how often a running run's log is saved. Zero
	// means logFlushInterval.
	LogFlushInterval time.Duration
	// ScheduleCheckInterval is how often RunSchedules looks for due runs.
	// Zero means scheduleCheckInterval.
	ScheduleCheckInterval time.Duration
}

// Runner discovers collectors and runs them in the background, one run per
// collector at a time.
type Runner struct {
	store *store.CollectorStore
	cfg   Config

	// reloadMu serializes scans of the collectors folder.
	reloadMu sync.Mutex

	mu     sync.Mutex
	cancel map[uuid.UUID]context.CancelCauseFunc
	wg     sync.WaitGroup

	events *broadcaster
}

// NewRunner returns a Runner that keeps its state in cs.
func NewRunner(cs *store.CollectorStore, cfg Config) *Runner {
	return &Runner{
		store: cs, cfg: cfg, cancel: map[uuid.UUID]context.CancelCauseFunc{}, events: newBroadcaster(),
	}
}

// Subscribe returns a channel that receives a run, without its issues and
// log, whenever it starts, changes phase, saves more of its log, or ends.
// The channel is closed after unsubscribe, or early when the subscriber
// falls behind; it then has missed updates and should reload the runs.
func (r *Runner) Subscribe() (runs <-chan store.CollectorRun, unsubscribe func()) {
	return r.events.subscribe()
}

// Reload scans the collectors folder, records what it found, and returns
// every known collector.
func (r *Runner) Reload(ctx context.Context) ([]store.Collector, error) {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	found, err := Discover(ctx, r.cfg.Dir, DescribeTimeout)
	if err != nil {
		return nil, err
	}

	if err := r.store.SaveDiscovered(ctx, found); err != nil {
		return nil, err
	}

	return r.store.List(ctx)
}

// Start begins a run of the collector and returns it while it runs. It
// returns store.ErrNotFound, ErrUnavailable, a *MissingSettingsError, or
// store.ErrRunActive.
func (r *Runner) Start(ctx context.Context, collectorID, triggeredBy string) (store.CollectorRun, error) {
	c, err := r.store.Get(ctx, collectorID)
	if err != nil {
		return store.CollectorRun{}, err
	}

	if c.Status != store.CollectorOK {
		return store.CollectorRun{}, ErrUnavailable
	}

	if missing := c.MissingRequiredSettings(); len(missing) > 0 {
		return store.CollectorRun{}, &MissingSettingsError{Keys: missing}
	}

	run, err := r.store.CreateRun(ctx, collectorID, triggeredBy)
	if err != nil {
		return store.CollectorRun{}, err
	}

	// The run outlives the request that started it.
	runCtx, cancel := context.WithCancelCause(context.WithoutCancel(ctx))

	r.mu.Lock()
	r.cancel[run.ID] = cancel
	r.mu.Unlock()

	r.events.publish(run)

	r.wg.Go(func() {
		defer func() {
			r.mu.Lock()
			delete(r.cancel, run.ID)
			r.mu.Unlock()
			cancel(nil)
		}()

		r.execute(runCtx, run, c)
	})

	return run, nil
}

// Cancel stops the running run with the given ID: its process is killed, or
// its sync rolled back, and nothing is applied.
func (r *Runner) Cancel(runID uuid.UUID) error {
	r.mu.Lock()
	cancel, ok := r.cancel[runID]
	r.mu.Unlock()

	if !ok {
		return ErrRunNotActive
	}

	cancel(errCancelled)

	return nil
}

// Wait blocks until every started run has finished.
func (r *Runner) Wait() {
	r.wg.Wait()
}

// execute runs the collector, syncs its output and records the outcome.
func (r *Runner) execute(ctx context.Context, run store.CollectorRun, c store.Collector) {
	outcome := r.collectAndSync(ctx, run, c)

	if errors.Is(context.Cause(ctx), errCancelled) {
		outcome.Status, outcome.Stats, outcome.Issues = store.RunCancelled, nil, nil
		outcome.Error = new("cancelled")
	}

	// Recording the outcome must not be cancelled with the run.
	finished, err := r.store.FinishRun(context.WithoutCancel(ctx), run.ID, outcome, r.cfg.RunHistory)
	if err != nil {
		log.Printf("collector %s: record run %s: %v", c.ID, run.ID, err)
		return
	}

	r.events.publish(finished)

	log.Printf("collector %s: run %s %s", c.ID, run.ID, outcome.Status)
}

func (r *Runner) collectAndSync(ctx context.Context, run store.CollectorRun, c store.Collector) store.RunOutcome {
	failed := func(format string, args ...any) store.RunOutcome {
		return store.RunOutcome{Status: store.RunFailed, Error: new(fmt.Sprintf(format, args...))}
	}

	env, err := collectEnv(baseEnv(os.LookupEnv, c.ID), run.ID, c.SettingsSchema, c.Settings)
	if err != nil {
		return failed("%v", err)
	}

	timeout := fmt.Errorf("timed out after %s", r.cfg.Timeout)
	collectCtx, cancel := context.WithTimeoutCause(ctx, r.cfg.Timeout, timeout)
	logBuf := &tailBuffer{max: maxLogBytes}
	stopLog := r.streamLog(ctx, run, logBuf)
	result, err := runCommand(collectCtx, c.Path, []string{"collect"}, env, logBuf)

	// The log is final once the process is gone; FinishRun saves it. Waiting
	// for the last save keeps it from racing the phase change and FinishRun.
	stopLog()
	cancel()

	outcome := store.RunOutcome{Log: result.log}
	if result.exitCode >= 0 {
		outcome.ExitCode = &result.exitCode
	}

	switch {
	case err != nil:
		outcome.Status, outcome.Error = store.RunFailed, new(err.Error())
		return outcome
	case result.exitCode != 0:
		outcome.Status, outcome.Error = store.RunFailed, new(fmt.Sprintf("the collector exited with status %d", result.exitCode))
		return outcome
	}

	out, err := contract.DecodeOutput(result.stdout)
	if err != nil {
		outcome.Status, outcome.Error = store.RunFailed, new(err.Error())
		return outcome
	}

	if err := r.store.SetRunPhase(ctx, run.ID, store.PhaseApplying); err != nil {
		log.Printf("collector %s: run %s: record phase: %v", c.ID, run.ID, err)
	} else {
		run.Phase = new(store.PhaseApplying)
		r.events.publish(run)
	}

	stats, issues, err := r.store.Sync(ctx, c.ID, out, store.SyncOptions{GeoMaxDistanceMeters: r.cfg.GeoMaxDistanceMeters})
	if err != nil {
		outcome.Status, outcome.Error = store.RunFailed, new("apply the output: "+err.Error())
		return outcome
	}

	outcome.Status, outcome.Stats, outcome.Issues = store.RunSucceeded, &stats, issues

	return outcome
}

// streamLog saves buf as the log of the running run every flush interval
// while it changes, so the log can be read before the run ends, and tells
// subscribers each time. The returned func stops it and waits until it has
// stopped.
func (r *Runner) streamLog(ctx context.Context, run store.CollectorRun, buf *tailBuffer) (stop func()) {
	interval := r.cfg.LogFlushInterval
	if interval <= 0 {
		interval = logFlushInterval
	}

	done := make(chan struct{})

	var wg sync.WaitGroup

	wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		var saved uint64

		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			s, writes := buf.Snapshot()
			if writes == saved {
				continue
			}

			if err := r.store.SetRunLog(ctx, run.ID, s); err != nil {
				log.Printf("run %s: save the log: %v", run.ID, err)
				continue
			}

			saved = writes

			r.events.publish(run)
		}
	})

	return func() {
		close(done)
		wg.Wait()
	}
}
