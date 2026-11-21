//go:build unix

package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validDescribe = `echo '{"contractVersion":1,"name":"Example","version":"1"}'`

func TestDiscover(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeScript(t, dir, "good.sh", validDescribe)
	writeScript(t, dir, "twice.sh", validDescribe)
	writeScript(t, dir, "twice.py", validDescribe)
	writeScript(t, dir, "Bad Name", validDescribe)
	writeScript(t, dir, "fails", "echo broken >&2; exit 2")
	writeScript(t, dir, "garbage", "echo nope")
	writeScript(t, dir, "slow", "sleep 30")
	writeScript(t, dir, ".hidden", validDescribe)

	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(filepath.Join(dir, "good.sh"), filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}

	found, err := Discover(t.Context(), dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}

	for _, d := range found {
		switch {
		case d.Description != nil && d.Error == "":
			got[d.ID] = "ok"
		case d.Description == nil && d.Error != "":
			got[d.ID] = d.Error
		default:
			t.Errorf("%s: description %v with error %q", d.ID, d.Description, d.Error)
		}
	}

	want := map[string]string{
		"good":     "ok",
		"linked":   "ok",
		"twice":    "several files share this ID: twice.py, twice.sh",
		"Bad Name": "must match",
		"fails":    "describe exited with status 2: broken",
		"garbage":  "parse describe output",
		"slow":     "describe timed out",
	}

	if len(got) != len(want) {
		t.Errorf("found %v, want %d collectors", got, len(want))
	}

	for id, w := range want {
		if !strings.Contains(got[id], w) {
			t.Errorf("%s = %q, want it to contain %q", id, got[id], w)
		}
	}
}

func TestDiscover_MissingFolder(t *testing.T) {
	t.Parallel()

	found, err := Discover(t.Context(), filepath.Join(t.TempDir(), "absent"), time.Second)
	if err != nil || len(found) != 0 {
		t.Errorf("Discover = %v, %v; want nothing", found, err)
	}
}
