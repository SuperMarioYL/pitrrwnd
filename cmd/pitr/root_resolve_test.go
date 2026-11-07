package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SuperMarioYL/pitrrwnd/internal/snapshot"
	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// TestSymlinkRootCapturesWorkingSet pins v0.4.0's root-identity fix: a --root
// reached through a symlink must snapshot the real directory. Before the fix
// filepath.Walk did not follow the root, so the savepoint "succeeded" with an
// empty manifest (working_set_sha256 was the hash of the empty string) and
// verify agreed because its manifest and its walk were both empty.
func TestSymlinkRootCapturesWorkingSet(t *testing.T) {
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(real, "app.conf"), []byte("port: 8080\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	link := filepath.Join(t.TempDir(), "linked-workspace")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink workspace: %v", err)
	}

	workspaceRoot = link
	if err := resolveWorkspaceRoot(); err != nil {
		t.Fatalf("resolveWorkspaceRoot: %v", err)
	}
	if workspaceRoot != real {
		t.Fatalf("resolved root = %q, want %q", workspaceRoot, real)
	}

	if err := runInit(nil, nil); err != nil {
		t.Fatalf("init through a symlinked root: %v", err)
	}
	savepointLabel = "linked"
	if err := runSavepoint(nil, nil); err != nil {
		t.Fatalf("savepoint through a symlinked root: %v", err)
	}

	st, err := store.Open(workspaceRoot)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	m, err := snapshot.LoadManifest(st, 1)
	if err != nil {
		_ = st.Close()
		t.Fatalf("load manifest: %v", err)
	}
	// The CLI commands below open the bbolt DB themselves; release the handle.
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if len(m.Files) != 1 || m.Files["app.conf"] == "" {
		t.Fatalf("manifest files = %v, want app.conf — a symlinked root captured nothing", m.Files)
	}

	// Break the file and prove the full cycle still works.
	if err := os.WriteFile(filepath.Join(real, "app.conf"), []byte("BROKEN\n"), 0o644); err != nil {
		t.Fatalf("break file: %v", err)
	}
	verifyStep = 1
	if err := runVerify(nil, nil); err == nil {
		t.Fatalf("verify should fail on the broken file")
	}
	rewindStep = 1
	if err := runRewind(nil, nil); err != nil {
		t.Fatalf("rewind: %v", err)
	}
	if err := runVerify(nil, nil); err != nil {
		t.Fatalf("verify after rewind: %v", err)
	}
}
