package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const runDirPrefix = "run-"

// newRunDir creates a fresh, uniquely-named scratch directory for this
// launch under base/runs/. Each run gets its own directory so the pristine
// payload disk image is never mutated in place.
func newRunDir(base string) (string, error) {
	runsDir := filepath.Join(base, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return "", fmt.Errorf("creating runs dir: %w", err)
	}
	dir, err := os.MkdirTemp(runsDir, runDirPrefix)
	if err != nil {
		return "", fmt.Errorf("creating run scratch dir: %w", err)
	}
	return dir, nil
}

// sweepStaleRuns best-effort deletes leftover run-* directories from
// crashed or forcibly-killed prior launches. A directory whose disk.img is
// still held open by a live RVVM process will fail to delete on Windows
// (sharing violation) and is silently left for a future sweep - this is
// simpler and more robust than tracking PIDs, since it relies on the OS's
// own file locking rather than us re-implementing liveness checks.
func sweepStaleRuns(base, skip string) {
	runsDir := filepath.Join(base, "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return // nothing to sweep, or runs dir doesn't exist yet
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), runDirPrefix) {
			continue
		}
		full := filepath.Join(runsDir, e.Name())
		if full == skip {
			continue
		}
		_ = os.RemoveAll(full) // best-effort; ignore failures (still in use, or already gone)
	}
}
