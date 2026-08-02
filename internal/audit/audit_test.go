package audit

import (
	"os"
	"strings"
	"testing"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

func TestAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()

	entries := []Entry{
		NewEntry(OpInit),
		{Op: OpSavepoint, Step: 1, Label: "before", WorkingSetSha256: "abc123"},
		{Op: OpRewind, Step: 1, Label: "before", WorkingSetSha256: "abc123", Result: "ok"},
		NewEntry(OpVerify),
	}
	for _, e := range entries {
		if err := Append(st, e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := Read(st)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("len = %d, want %d", len(got), len(entries))
	}
	if got[1].Step != 1 || got[1].Label != "before" {
		t.Fatalf("entry 1 = %+v", got[1])
	}
	for _, e := range got {
		if e.IsoTs == "" || e.Hostname == "" || e.User == "" {
			t.Fatalf("entry missing audit field: %+v", e)
		}
	}
}

func TestHash(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()

	h1, err := Hash(st)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if err := Append(st, NewEntry(OpSavepoint)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	h2, err := Hash(st)
	if err != nil {
		t.Fatalf("Hash2: %v", err)
	}
	if h1 == h2 {
		t.Fatalf("hash unchanged after append: %s", h1)
	}
	if len(h2) != 64 {
		t.Fatalf("hash len = %d, want 64", len(h2))
	}
}

// TestIntegrity guards against silent ledger corruption being skipped.
func TestIntegrity(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()
	// Append a malformed line directly to the ledger file.
	if err := Append(st, NewEntry(OpInit)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := appendRaw(st, "not-json\n"); err != nil {
		t.Fatalf("appendRaw: %v", err)
	}
	if _, err := Read(st); err == nil || !strings.Contains(err.Error(), "line") {
		t.Fatalf("Read on malformed ledger should error, got %v", err)
	}
}

// appendRaw writes a raw line bypassing JSON validation, to test integrity.
func appendRaw(st *store.Store, line string) error {
	f, err := os.OpenFile(st.AuditPath(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}
