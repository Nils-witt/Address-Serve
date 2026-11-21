//go:build unix

package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// writeScript writes an executable shell script and returns its path.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	//nolint:gosec // a collector script has to be executable
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestRunCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		wantOut  string
		wantLog  string
		wantExit int
	}{
		{name: "output and log", body: "echo out; echo log >&2", wantOut: "out\n", wantLog: "log\n"},
		{name: "non-zero exit is not an error", body: "echo failing >&2; exit 3", wantLog: "failing\n", wantExit: 3},
		{name: "arguments", body: `echo "$1"`, wantOut: "collect\n"},
		{name: "fresh working directory", body: `ls -A; case "$PWD" in *address-serv-collector-*) echo tmp;; esac`, wantOut: "tmp\n"},
		{name: "only the given env", body: `echo "${DATABASE_URL:-unset} $ONLY"`, wantOut: "unset given\n"},
		{name: "no stdin", body: "cat", wantOut: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := writeScript(t, t.TempDir(), "c", tt.body)

			result, err := runCommand(t.Context(), path, []string{"collect"}, []string{"PATH=/bin:/usr/bin", "ONLY=given"})
			if err != nil {
				t.Fatal(err)
			}

			if string(result.stdout) != tt.wantOut || result.log != tt.wantLog || result.exitCode != tt.wantExit {
				t.Errorf("result = %q, %q, %d; want %q, %q, %d",
					result.stdout, result.log, result.exitCode, tt.wantOut, tt.wantLog, tt.wantExit)
			}
		})
	}
}

func TestRunCommand_StartFailure(t *testing.T) {
	t.Parallel()

	if _, err := runCommand(t.Context(), filepath.Join(t.TempDir(), "missing"), nil, nil); err == nil {
		t.Error("runCommand of a missing file succeeded")
	}
}

// TestRunCommand_KillsProcessGroup checks that neither a timeout nor a normal
// exit leaves processes the collector started behind.
func TestRunCommand_KillsProcessGroup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		timeout time.Duration
		wantErr bool
	}{
		{name: "on timeout", body: "sleep 60 & echo $! > \"$PIDFILE\"; sleep 60", timeout: 300 * time.Millisecond, wantErr: true},
		{name: "after exit", body: "sleep 60 >/dev/null 2>&1 & echo $! > \"$PIDFILE\"", timeout: time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			pidFile := filepath.Join(dir, "pid")
			path := writeScript(t, dir, "c", tt.body)
			cause := errors.New("too slow")

			ctx, cancel := context.WithTimeoutCause(t.Context(), tt.timeout, cause)
			defer cancel()

			start := time.Now()
			_, err := runCommand(ctx, path, nil, []string{"PATH=/bin:/usr/bin", "PIDFILE=" + pidFile})

			if tt.wantErr != errors.Is(err, cause) {
				t.Fatalf("err = %v, want cause %v: %t", err, cause, tt.wantErr)
			}

			if time.Since(start) > 10*time.Second {
				t.Errorf("runCommand took %s", time.Since(start))
			}

			waitGone(t, pidFile)
		})
	}
}

// waitGone fails t unless the process whose ID is in pidFile is gone soon.
func waitGone(t *testing.T, pidFile string) {
	t.Helper()

	data, err := os.ReadFile(pidFile) //nolint:gosec // the test's own temporary file
	if err != nil {
		t.Fatal(err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}

	for range 50 {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	_ = syscall.Kill(pid, syscall.SIGKILL)

	t.Errorf("child process %d is still running", pid)
}
