package collector

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nils-witt/address-serve/internal/collector/contract"
	"github.com/nils-witt/address-serve/internal/store"
)

// idPattern is what a collector ID, its filename up to the first dot, must
// look like.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// maxErrorLog caps the stderr quoted in a describe error.
const maxErrorLog = 500

// Discover scans dir for collectors and describes each one, allowing each
// describeTimeout. Problems with single files make them invalid collectors
// rather than errors; only an unreadable folder is an error, and a missing one
// holds no collectors.
func Discover(ctx context.Context, dir string, describeTimeout time.Duration) ([]store.Discovered, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read collectors folder: %w", err)
	}

	byID := map[string][]string{}

	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if strings.HasPrefix(e.Name(), ".") || !isCollectorFile(path) {
			continue
		}

		id, _, _ := strings.Cut(e.Name(), ".")
		byID[id] = append(byID[id], path)
	}

	found := make([]store.Discovered, 0, len(byID))

	var wg sync.WaitGroup

	for _, id := range slices.Sorted(maps.Keys(byID)) {
		paths := byID[id]
		found = append(found, store.Discovered{ID: id, Path: paths[0]})
		d := &found[len(found)-1]

		switch {
		case !idPattern.MatchString(id):
			d.Error = fmt.Sprintf("the ID %q, the filename up to its first dot, must match %s", id, idPattern)
		case len(paths) > 1:
			d.Error = "several files share this ID: " + strings.Join(baseNames(paths), ", ")
		default:
			wg.Go(func() { d.Description, d.Error = describe(ctx, id, d.Path, describeTimeout) })
		}
	}

	wg.Wait()

	return found, nil
}

// isCollectorFile reports whether path, following symlinks, is a regular
// file the app may execute.
func isCollectorFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && isExecutable(path)
}

// describe runs `<path> describe` and returns its validated output, or why
// it was rejected.
func describe(ctx context.Context, id, path string, timeout time.Duration) (*contract.Description, string) {
	ctx, cancel := context.WithTimeoutCause(ctx, timeout, fmt.Errorf("describe timed out after %s", timeout))
	defer cancel()

	result, err := runCommand(ctx, path, []string{"describe"}, baseEnv(os.LookupEnv, id), nil)
	if err != nil {
		return nil, err.Error()
	}

	if result.exitCode != 0 {
		return nil, fmt.Sprintf("describe exited with status %d%s", result.exitCode, quoteLog(result.log))
	}

	d, err := contract.DecodeDescription(result.stdout)
	if err != nil {
		return nil, err.Error()
	}

	return &d, ""
}

// quoteLog formats the end of a failed command's stderr for an error message.
func quoteLog(log string) string {
	log = strings.TrimSpace(log)
	if log == "" {
		return ""
	}

	if len(log) > maxErrorLog {
		log = "…" + log[len(log)-maxErrorLog:]
	}

	return ": " + log
}

func baseNames(paths []string) []string {
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
	}

	return names
}
