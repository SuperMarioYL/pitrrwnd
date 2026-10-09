package redo

import (
	"path/filepath"
	"testing"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

func TestAppendReadTruncate(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()
	redoPath := st.RedoLogPath()

	if err := Append(redoPath, NewEntry(OpWrite, "a.txt", "abc123", 3)); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if err := Append(redoPath, NewEntry(OpCreate, "b.txt", "def456", 3)); err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	got, err := Read(redoPath)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 2 || got[0].Path != "a.txt" || got[1].Op != OpCreate {
		t.Fatalf("entries = %+v", got)
	}
	for _, e := range got {
		if e.IsoTs == "" {
			t.Fatalf("entry missing timestamp: %+v", e)
		}
	}

	if err := Truncate(redoPath); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	got2, err := Read(redoPath)
	if err != nil {
		t.Fatalf("Read after truncate: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("redo log not empty after truncate: %d", len(got2))
	}
}

func TestAppendCreatesMissingLog(t *testing.T) {
	// Append must work on a log that does not exist yet (e.g. a deleted
	// redo.wal): the watcher is best-effort and must not crash.
	log := filepath.Join(t.TempDir(), "redo.wal")
	if err := Append(log, NewEntry(OpWrite, "a.txt", "abc123", 3)); err != nil {
		t.Fatalf("Append on missing log: %v", err)
	}
	got, err := Read(log)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 || got[0].Path != "a.txt" {
		t.Fatalf("entries = %+v", got)
	}
}
