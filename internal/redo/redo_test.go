package redo

import (
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

	if err := Append(st, NewEntry(OpWrite, "a.txt", "abc123", 3)); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if err := Append(st, NewEntry(OpCreate, "b.txt", "def456", 3)); err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	got, err := Read(st)
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

	if err := Truncate(st); err != nil {
		t.Fatalf("Truncate: %v", err)
	}
	got2, err := Read(st)
	if err != nil {
		t.Fatalf("Read after truncate: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("redo log not empty after truncate: %d", len(got2))
	}
}
