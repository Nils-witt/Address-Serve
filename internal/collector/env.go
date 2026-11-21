// Package collector finds collector executables, runs them and syncs their
// output into the store. The contract between the app and a collector is in
// docs/collectors/contract.md and the contract subpackage.
package collector

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/nils-witt/address-serve/internal/collector/contract"
)

// passthroughEnv lists the variables a collector inherits from the app. All
// others stay hidden: the app's environment holds database credentials.
var passthroughEnv = []string{
	"PATH", "HOME", "TMPDIR", "LANG", "TZ",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR",
}

// baseEnv returns the environment of every collector command: the inherited
// variables lookup finds, plus the contract version and collector ID.
func baseEnv(lookup func(string) (string, bool), collectorID string) []string {
	env := make([]string, 0, len(passthroughEnv)+2)

	for _, key := range passthroughEnv {
		if v, ok := lookup(key); ok {
			env = append(env, key+"="+v)
		}
	}

	return append(env,
		"ADDRESS_SERV_CONTRACT_VERSION="+strconv.Itoa(contract.Version),
		"ADDRESS_SERV_COLLECTOR_ID="+collectorID,
	)
}

// collectEnv returns the environment of `collect`: base plus the run ID and
// one variable per setting that has a value or default.
func collectEnv(
	base []string, runID uuid.UUID, schema []contract.SettingSpec, settings map[string]json.RawMessage,
) ([]string, error) {
	env := append(append([]string{}, base...), "ADDRESS_SERV_RUN_ID="+runID.String())

	for _, spec := range schema {
		value := settings[spec.Key]
		if value == nil {
			value = spec.Default
		}

		if value == nil {
			continue
		}

		rendered, err := spec.EnvValue(value)
		if err != nil {
			return nil, fmt.Errorf("setting %s: %w", spec.Key, err)
		}

		env = append(env, spec.EnvName()+"="+rendered)
	}

	return env, nil
}

// maxLogBytes caps the stderr kept from a collector run.
const maxLogBytes = 1 << 20

// logFlushInterval is how often a running run's log is saved, so the UI can
// show it while the run goes on. It keeps pace with the UI's polling without
// rewriting a log of up to maxLogBytes much more often than anyone reads it.
const logFlushInterval = 2 * time.Second

// tailBuffer keeps the last max bytes written to it. It is safe for
// concurrent use: the command writes stderr while the run's log is saved.
type tailBuffer struct {
	max int

	mu        sync.Mutex
	buf       []byte
	truncated bool
	// writes counts the writes, so a reader can tell whether anything
	// changed since it last looked.
	writes uint64
}

// Write implements io.Writer. It never fails.
func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.buf = append(t.buf, p...)
	t.writes++

	if over := len(t.buf) - t.max; over > 0 {
		// Copying keeps the buffer from growing without bound.
		t.buf = append(t.buf[:0:0], t.buf[over:]...)
		t.truncated = true
	}

	return len(p), nil
}

// String returns the kept bytes as text PostgreSQL can store: valid UTF-8
// without NUL bytes.
func (t *tailBuffer) String() string {
	s, _ := t.Snapshot()

	return s
}

// Snapshot returns what String returns along with the number of writes so
// far.
func (t *tailBuffer) Snapshot() (string, uint64) {
	t.mu.Lock()
	raw, truncated, writes := string(t.buf), t.truncated, t.writes
	t.mu.Unlock()

	s := strings.ReplaceAll(strings.ToValidUTF8(raw, string(utf8.RuneError)), "\x00", "")
	if truncated {
		return "[earlier output truncated]\n" + s, writes
	}

	return s, writes
}
