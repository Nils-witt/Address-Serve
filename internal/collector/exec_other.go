//go:build !unix

package collector

import (
	"context"
	"errors"
)

// Supported reports whether collectors can run on this OS.
const Supported = false

var errUnsupported = errors.New("collectors are only supported on Unix-like systems")

type commandResult struct {
	stdout   []byte
	log      string
	exitCode int
}

func runCommand(context.Context, string, []string, []string, *tailBuffer) (commandResult, error) {
	return commandResult{}, errUnsupported
}

func isExecutable(string) bool { return false }
