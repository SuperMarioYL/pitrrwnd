package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/SuperMarioYL/pitrrwnd/internal/redo"
	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// startWatch launches Watch on the redo path the way runWatch does (store
// opened only long enough to resolve the path, then closed so the bbolt lock
// is released) and returns a cancel func plus the done channel.
func startWatch(t *testing.T, dir, redoPath string) (context.CancelFunc, chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Watch(ctx, dir, redoPath, nil) }()
	return cancel, done
}

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
	redoPath := st.RedoLogPath()
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	cancel, done := startWatch(t, dir, redoPath)

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

	waitFor(t, 600*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("watcher did not stop")
	}

	entries, err := redo.Read(redoPath)
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

// TestWatchDoesNotHoldStateDBLock is the regression test for v0.3.0's m7 fix:
// the watcher must run without holding the bbolt state.db handle, or its
// exclusive lock times out every concurrent savepoint/rewind/verify/log (the
// exact workflow the plan documents: watch in the foreground while stateless
// commands run in another terminal).
func TestWatchDoesNotHoldStateDBLock(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	redoPath := st.RedoLogPath()
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	cancel, done := startWatch(t, dir, redoPath)
	waitFor(t, 300*time.Millisecond)

	// Exactly what every stateless command does while the watcher is live:
	// open the state DB and reserve a savepoint step. With the lock held this
	// fails after bolt's 3s timeout with "open state db: timeout".
	st2, err := store.Open(dir)
	if err != nil {
		cancel()
		t.Fatalf("concurrent store.Open while watching (lock leaked?): %v", err)
	}
	defer st2.Close()
	step, err := st2.AllocStep()
	if err != nil {
		t.Fatalf("concurrent AllocStep: %v", err)
	}
	if step != 1 {
		t.Fatalf("AllocStep = %d, want 1", step)
	}
	sp := store.Savepoint{Step: step, Label: "concurrent", Actor: "tester", CreatedAt: time.Now().UTC()}
	if err := st2.SaveSavepoint(sp); err != nil {
		t.Fatalf("concurrent SaveSavepoint: %v", err)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("watcher did not stop")
	}
}

// TestWatchSkipsPitrEvents pins the filter behaviour at the handle level.
func TestWatchSkipsPitrEvents(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	e := handle(st.RedoLogPath(), dir, w, fsnotify.Event{
		Name: filepath.Join(dir, store.PitrDirName, "state.db"),
		Op:   fsnotify.Write,
	})
	if e.Path != "" {
		t.Fatalf("event inside .pitr must be skipped, got %+v", e)
	}
}

func waitFor(t *testing.T, d time.Duration) {
	t.Helper()
	time.Sleep(d)
}
