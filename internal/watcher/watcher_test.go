package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SuperMarioYL/pitrrwnd/internal/redo"
	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// TestWatchAppendsRedoEntries starts the watcher, writes a file, stops, and
// asserts a redo entry was recorded. Filesystem-watch timing makes this
// inherently racy; we allow generous sleeps and assert only that >=1 entry
// exists, not an exact count.
func TestWatchAppendsRedoEntries(t *testing.T) {
	dir := t.TempDir()
	// A baseline file present before watch starts (must not crash the watcher).
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Watch(ctx, dir, st, nil) }()

	// Let the watcher seed its watch tree.
	waitFor(t, 500*time.Millisecond)

	// Simulate an agent write + a new nested dir write.
	if err := os.WriteFile(filepath.Join(dir, "app.conf"), []byte("port=1\n"), 0o644); err != nil {
		cancel()
		t.Fatalf("write: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0o755); err != nil {
		cancel()
		t.Fatalf("mkdir: %v", err)
	}
	waitFor(t, 200*time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "nested/x.txt"), []byte("nested\n"), 0o644); err != nil {
		cancel()
		t.Fatalf("nested write: %v", err)
	}

	waitFor(t, 600 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("watcher did not stop")
	}

	entries, err := redo.Read(st)
	if err != nil {
		t.Fatalf("redo Read: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected >=1 redo entry, got 0")
	}
	// No entry should reference the state directory.
	for _, e := range entries {
		if e.Path == store.PitrDirName || len(e.Path) >= len(store.PitrDirName) && e.Path[:len(store.PitrDirName)] == store.PitrDirName {
			t.Fatalf("redo leaked .pitr path: %q", e.Path)
		}
	}
}

func waitFor(t *testing.T, d time.Duration) {
	t.Helper()
	time.Sleep(d)
}
