package store

import (
	"path/filepath"
	"testing"
)

func TestInitAndOpen(t *testing.T) {
	dir := t.TempDir()
	st, err := Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := filepath.Abs(dir); err != nil {
		t.Fatalf("abs: %v", err)
	}
	// Re-open an initialized workspace.
	st2, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st2.Close()
	if st2.PitrDir != filepath.Join(dir, PitrDirName) {
		t.Fatalf("pitr dir = %q", st2.PitrDir)
	}
}

func TestOpenUninitialized(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != ErrNotInitialized {
		t.Fatalf("Open on empty dir: want ErrNotInitialized, got %v", err)
	}
}

func TestAllocStepAndSaveSavepoint(t *testing.T) {
	dir := t.TempDir()
	st, err := Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()

	s1, err := st.AllocStep()
	if err != nil {
		t.Fatalf("AllocStep 1: %v", err)
	}
	s2, err := st.AllocStep()
	if err != nil {
		t.Fatalf("AllocStep 2: %v", err)
	}
	if s1 != 1 || s2 != 2 {
		t.Fatalf("steps = %d,%d, want 1,2", s1, s2)
	}

	for _, sp := range []Savepoint{{Step: 1, Label: "a"}, {Step: 2, Label: "b"}} {
		if err := st.SaveSavepoint(sp); err != nil {
			t.Fatalf("SaveSavepoint: %v", err)
		}
	}
	got, err := st.ListSavepoints()
	if err != nil {
		t.Fatalf("ListSavepoints: %v", err)
	}
	if len(got) != 2 || got[0].Step != 1 || got[1].Step != 2 {
		t.Fatalf("list = %+v", got)
	}
	got1, err := st.GetSavepoint(1)
	if err != nil || got1.Label != "a" {
		t.Fatalf("GetSavepoint(1) = %+v err=%v", got1, err)
	}
}
