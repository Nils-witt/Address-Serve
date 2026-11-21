// Package collector finds collector executables, runs them and syncs their
// output into the store. The contract between the app and a collector is in
// docs/collectors/contract.md and the contract subpackage.
package collector

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	max       int
	buf       []byte
	truncated bool
}

// Write implements io.Writer. It never fails.
func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)

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
	s := strings.ReplaceAll(strings.ToValidUTF8(string(t.buf), string(utf8.RuneError)), "\x00", "")
	if t.truncated {
		return "[earlier output truncated]\n" + s
	}

	return s
}
