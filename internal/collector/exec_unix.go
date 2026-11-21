//go:build unix

package collector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Supported reports whether collectors can run on this OS.
const Supported = true

// waitDelay bounds how long a finished collector's leftover children may
// hold its output pipes open before they are closed.
const waitDelay = 5 * time.Second

// commandResult is what a collector command produced.
type commandResult struct {
	stdout   []byte
	log      string
	exitCode int
}

// runCommand runs path with args in a fresh temporary directory, with env as
// its whole environment and no stdin. Its stderr goes to stderr, or to a
// fresh buffer if that is nil, and ends up in the result's log. It runs in its own process group,
// which is killed when ctx ends and once the command exits, so children it
// started don't outlive it. A non-zero exit is not an error; failing to
// start and ctx ending are.
func runCommand(ctx context.Context, path string, args, env []string, stderr *tailBuffer) (commandResult, error) {
	dir, err := os.MkdirTemp("", "address-serv-collector-*")
	if err != nil {
		return commandResult{}, fmt.Errorf("create working directory: %w", err)
	}

	defer func() { _ = os.RemoveAll(dir) }()

	var stdout bytes.Buffer

	if stderr == nil {
		stderr = &tailBuffer{max: maxLogBytes}
	}

	// The path comes from the collectors folder, which only the operator
	// controls; running what's there is the point.
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // see above
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdout, cmd.Stderr = &stdout, stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd.Process.Pid) }
	cmd.WaitDelay = waitDelay

	waitErr := cmd.Run()

	if cmd.Process == nil {
		return commandResult{}, fmt.Errorf("start %s: %w", path, waitErr)
	}

	// Children the collector left behind go with it.
	_ = killGroup(cmd.Process.Pid)

	result := commandResult{stdout: stdout.Bytes(), log: stderr.String(), exitCode: cmd.ProcessState.ExitCode()}

	if ctx.Err() != nil {
		return result, context.Cause(ctx)
	}

	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) && !errors.Is(waitErr, exec.ErrWaitDelay) {
		return result, waitErr
	}

	return result, nil
}

// killGroup kills the process group led by pid.
func killGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}

// isExecutable reports whether the app may execute the file at path.
func isExecutable(path string) bool {
	const executeOK = 0x1 // X_OK

	return syscall.Access(path, executeOK) == nil
}
