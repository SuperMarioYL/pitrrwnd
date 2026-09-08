package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// initWorkspace creates a .pitr/ workspace in dir with one working file and
// points the package-level flag variables at it, so the runX helpers can be
// exercised exactly as the CLI would drive them. Tests here must not run in
// parallel: they share those globals.
func initWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(p, []byte("port: 8080\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("store.Init: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("store.Close: %v", err)
	}
	workspaceRoot = dir
	return dir
}

// breakAuditLedger makes every audit append deterministically fail by
// replacing the ledger file with a directory: opening a directory with
// O_WRONLY fails for every user, unlike a chmod which root ignores.
func breakAuditLedger(t *testing.T, dir string) {
	t.Helper()
	ledger := filepath.Join(dir, ".pitr", "audit.jsonl")
	if err := os.Remove(ledger); err != nil {
		t.Fatalf("remove ledger: %v", err)
	}
	if err := os.Mkdir(ledger, 0o755); err != nil {
		t.Fatalf("replace ledger with dir: %v", err)
	}
}

func TestCLISavepointSurfacesAuditFailure(t *testing.T) {
	dir := initWorkspace(t)
	breakAuditLedger(t, dir)

	err := runSavepoint(nil, nil)
	if err == nil {
		t.Fatalf("savepoint must fail when the audit ledger is unwritable; it silently recorded nothing")
	}
	if !strings.Contains(err.Error(), "audit ledger") {
		t.Fatalf("error should name the audit ledger, got: %v", err)
	}
}

func TestCLIRewindSurfacesAuditFailure(t *testing.T) {
	dir := initWorkspace(t)
	savepointLabel = "before"
	if err := runSavepoint(nil, nil); err != nil {
		t.Fatalf("savepoint (intact ledger): %v", err)
	}

	// Break the workspace like a 3am agent edit, then break the ledger.
	if err := os.WriteFile(filepath.Join(dir, "app.conf"), []byte("BROKEN\n"), 0o644); err != nil {
		t.Fatalf("break conf: %v", err)
	}
	breakAuditLedger(t, dir)

	rewindStep = 1
	err := runRewind(nil, nil)
	if err == nil {
		t.Fatalf("rewind must fail when the audit ledger is unwritable; the restore would be unaudited")
	}
	if !strings.Contains(err.Error(), "restored to step 1") || !strings.Contains(err.Error(), "audit ledger") {
		t.Fatalf("error should say the restore succeeded but the ledger append failed, got: %v", err)
	}

	// The restore itself must have happened despite the ledger failure.
	b, rerr := os.ReadFile(filepath.Join(dir, "app.conf"))
	if rerr != nil || string(b) != "port: 8080\n" {
		t.Fatalf("working set should be restored even when the ledger append fails: %q err=%v", b, rerr)
	}
}

func TestCLIVerifySurfacesAuditFailure(t *testing.T) {
	dir := initWorkspace(t)
	savepointLabel = "before"
	if err := runSavepoint(nil, nil); err != nil {
		t.Fatalf("savepoint (intact ledger): %v", err)
	}
	breakAuditLedger(t, dir)

	verifyStep = 1
	err := runVerify(nil, nil)
	if err == nil {
		t.Fatalf("verify must fail when the audit ledger is unwritable; the result would go unrecorded")
	}
	if !strings.Contains(err.Error(), "audit ledger") {
		t.Fatalf("error should name the audit ledger, got: %v", err)
	}
}

// TestCLICycleHappyPath drives the full savepoint -> break -> rewind -> verify
// cycle through the command layer with a writable ledger: everything exits nil.
func TestCLICycleHappyPath(t *testing.T) {
	dir := initWorkspace(t)
	savepointLabel = "before risky refactor"
	if err := runSavepoint(nil, nil); err != nil {
		t.Fatalf("savepoint: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "app.conf"), []byte("port: 9999\nBROKEN\n"), 0o644); err != nil {
		t.Fatalf("break conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-junk.txt"), []byte("junk\n"), 0o644); err != nil {
		t.Fatalf("junk file: %v", err)
	}

	verifyStep = 1
	if err := runVerify(nil, nil); err == nil {
		t.Fatalf("verify should fail on a broken working set")
	}

	rewindStep = 1
	if err := runRewind(nil, nil); err != nil {
		t.Fatalf("rewind: %v", err)
	}
	if err := runVerify(nil, nil); err != nil {
		t.Fatalf("verify after rewind should pass (killer demo): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "agent-junk.txt")); !os.IsNotExist(err) {
		t.Fatalf("agent junk should be gone after rewind")
	}
}
