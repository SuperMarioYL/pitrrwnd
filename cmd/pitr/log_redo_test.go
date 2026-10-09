package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperMarioYL/pitrrwnd/internal/redo"
	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// captureStdout redirects os.Stdout to a pipe for the duration of fn (runLog
// and runWatch print via fmt to os.Stdout) and returns everything written.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	fnErr := fn()
	os.Stdout = old
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close pipe reader: %v", err)
	}
	return string(data), fnErr
}

// seedRedo writes redo entries through the real append path.
func seedRedo(t *testing.T, dir string, entries ...redo.Entry) {
	t.Helper()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer st.Close()
	for _, e := range entries {
		if err := redo.Append(st.RedoLogPath(), e); err != nil {
			t.Fatalf("redo.Append: %v", err)
		}
	}
}

// TestLogShowsRedoSection pins m8: pitr log lists the pending redo entries
// (per-write events since the last savepoint) after the audit trail.
func TestLogShowsRedoSection(t *testing.T) {
	dir := initWorkspace(t)
	seedRedo(t, dir,
		redo.NewEntry(redo.OpWrite, "app.conf", "0123456789abcdef", 11),
		redo.NewEntry(redo.OpCreate, "agent-junk.txt", "fedcba9876543210", 5),
	)

	out, err := captureStdout(t, func() error { return runLog(nil, nil) })
	if err != nil {
		t.Fatalf("runLog: %v", err)
	}
	if !strings.Contains(out, "== Redo log (since last savepoint) ==") {
		t.Fatalf("log output missing redo section:\n%s", out)
	}
	if !strings.Contains(out, "app.conf") || !strings.Contains(out, "agent-junk.txt") {
		t.Fatalf("redo section missing seeded paths:\n%s", out)
	}
	if !strings.Contains(out, "01234567") || !strings.Contains(out, "write") || !strings.Contains(out, "create") {
		t.Fatalf("redo section missing op/sha8 detail:\n%s", out)
	}
	// The pre-existing sections stay intact.
	if !strings.Contains(out, "== Savepoints ==") || !strings.Contains(out, "== Audit trail ==") {
		t.Fatalf("log output lost savepoints/audit sections:\n%s", out)
	}
}

// TestLogWarnsOnTornRedoTail pins the best-effort contract: a torn redo log
// (possible since the watcher appends while savepoint truncates concurrently)
// warns instead of failing the whole log command, and the savepoints/audit
// sections stay intact. The audit ledger keeps its strict integrity contract.
func TestLogWarnsOnTornRedoTail(t *testing.T) {
	dir := initWorkspace(t)
	redoPath := filepath.Join(dir, ".pitr", "redo.wal")
	valid := `{"path":"app.conf","sha256":"0123456789abcdef","size":11,"op":"write","iso_ts":"2026-10-10T00:00:00Z"}` + "\n"
	if err := os.WriteFile(redoPath, []byte(valid+`{"path":"torn`), 0o644); err != nil {
		t.Fatalf("seed torn log: %v", err)
	}

	out, err := captureStdout(t, func() error { return runLog(nil, nil) })
	if err != nil {
		t.Fatalf("runLog must not fail on a torn redo tail: %v", err)
	}
	if !strings.Contains(out, "redo log unreadable") {
		t.Fatalf("log output should warn about the unreadable redo log:\n%s", out)
	}
	if !strings.Contains(out, "== Savepoints ==") || !strings.Contains(out, "== Audit trail ==") {
		t.Fatalf("torn redo tail must not remove other sections:\n%s", out)
	}
}

// TestLogEmptyRedoPlaceholder pins the empty-log UX.
func TestLogEmptyRedoPlaceholder(t *testing.T) {
	initWorkspace(t)

	out, err := captureStdout(t, func() error { return runLog(nil, nil) })
	if err != nil {
		t.Fatalf("runLog: %v", err)
	}
	if !strings.Contains(out, "(no pending writes") {
		t.Fatalf("log output missing empty-redo placeholder:\n%s", out)
	}
}
