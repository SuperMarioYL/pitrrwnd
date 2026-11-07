package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// writeFile writes a regular file under dir with the given relative path.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// TestTakeVerifyRewindCycle is the killer demo: snapshot, break, rewind,
// verify exits clean. It also asserts verify FAILS while broken.
func TestTakeVerifyRewindCycle(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()

	writeFile(t, dir, "app/conf.yaml", "port: 8080\n")
	writeFile(t, dir, "data/state.txt", "hello\n")
	writeFile(t, dir, "lib/util.go", "package lib\n")

	// Step 1: before the agent runs.
	sp, err := Take(dir, st, "before risky refactor", "tester")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	if sp.Step != 1 {
		t.Fatalf("step = %d, want 1", sp.Step)
	}
	if sp.WorkingSetSha256 == "" {
		t.Fatalf("empty working set sha")
	}

	// Verify passes right after a savepoint.
	if err := Verify(dir, st, 1); err != nil {
		t.Fatalf("Verify after savepoint should pass, got %v", err)
	}

	// Simulate the agent breaking the workspace: in-place edit + new file.
	writeFile(t, dir, "app/conf.yaml", "port: 9999\nBROKEN\n")
	if err := os.WriteFile(filepath.Join(dir, "data/state.txt"), []byte("corrupted\n"), 0o644); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	writeFile(t, dir, "agent/junk.txt", "the agent made this\n")

	// Verify must now FAIL.
	if err := Verify(dir, st, 1); err == nil {
		t.Fatalf("Verify after breakage should fail")
	}

	// Rewind to step 1.
	sp2, err := Restore(dir, st, 1)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if sp2.WorkingSetSha256 != sp.WorkingSetSha256 {
		t.Fatalf("working set sha after rewind = %s, want %s", sp2.WorkingSetSha256, sp.WorkingSetSha256)
	}

	// After rewind: agent junk gone, contents restored.
	if _, err := os.Stat(filepath.Join(dir, "agent/junk.txt")); !os.IsNotExist(err) {
		t.Fatalf("agent file should be gone after rewind")
	}
	b, err := os.ReadFile(filepath.Join(dir, "app/conf.yaml"))
	if err != nil || string(b) != "port: 8080\n" {
		t.Fatalf("conf not restored: %q err=%v", b, err)
	}

	// The killer demo: verify exits clean byte-level equal.
	if err := Verify(dir, st, 1); err != nil {
		t.Fatalf("Verify after rewind should pass, got %v", err)
	}
}

func TestLoadManifestMissing(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()
	if _, err := LoadManifest(st, 99); err == nil {
		t.Fatalf("LoadManifest missing should error")
	}
}

// TestTakeManifestMatchesSnapshotBytes locks the invariant the v0.1.0 TOCTOU
// broke: every manifest hash must describe the bytes actually stored in the
// snapshot, not a racy re-read of the working-set source. Restore() copies the
// snapshot files back, so only this invariant guarantees rewind -> verify is
// byte-level equal even when a file changed while the savepoint was taken.
func TestTakeManifestMatchesSnapshotBytes(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()

	writeFile(t, dir, "app/conf.yaml", "port: 8080\n")
	writeFile(t, dir, "data/state.txt", "hello\n")

	sp, err := Take(dir, st, "consistency", "tester")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	m, err := LoadManifest(st, sp.Step)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(m.Files) != 2 {
		t.Fatalf("manifest files = %d, want 2", len(m.Files))
	}
	for rel, want := range m.Files {
		got, err := hashFile(filepath.Join(st.SnapshotDir(sp.Step), rel))
		if err != nil {
			t.Fatalf("hash snapshot %s: %v", rel, err)
		}
		if got != want {
			t.Fatalf("manifest hash for %s does not describe the snapshot bytes (manifest %s, snapshot %s)", rel, want, got)
		}
	}
}

func TestWalkFilesSkipsPitr(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "a")
	st, _ := store.Init(dir, "tester")
	defer st.Close()
	writeFile(t, dir, ".pitr/state.db", "should-not-walk") // created by Init; ensure skipped

	files, err := walkFiles(dir)
	if err != nil {
		t.Fatalf("walkFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "a.txt" {
		t.Fatalf("files = %v", files)
	}
	for _, f := range files {
		if strings.HasPrefix(f, store.PitrDirName) {
			t.Fatalf(".pitr leaked into walk: %q", f)
		}
	}
}

// TestNestedPitrDirIsWorkingSetContent pins v0.4.0's state-directory rule: only
// root/.pitr is state. A nested directory that happens to be named .pitr is
// ordinary working-set content, so it must be snapshotted, restorable and
// verified — previously it was silently skipped and verify could report OK for a
// working set that had lost a file it had snapshotted.
func TestNestedPitrDirIsWorkingSetContent(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Init(dir, "tester")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer st.Close()

	writeFile(t, dir, "sub/.pitr/config.txt", "important\n")
	writeFile(t, dir, "sub/normal.txt", "nested\n")

	sp, err := Take(dir, st, "nested", "tester")
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	m, err := LoadManifest(st, sp.Step)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if _, ok := m.Files["sub/.pitr/config.txt"]; !ok {
		t.Fatalf("nested .pitr content is missing from the manifest: %v", m.Files)
	}

	// The agent deletes it: verify must notice, then rewind must restore it.
	if err := os.Remove(filepath.Join(dir, "sub", ".pitr", "config.txt")); err != nil {
		t.Fatalf("remove nested config: %v", err)
	}
	if err := Verify(dir, st, sp.Step); err == nil {
		t.Fatalf("Verify must fail when a snapshotted nested .pitr file is gone")
	}
	if _, err := Restore(dir, st, sp.Step); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "sub", ".pitr", "config.txt")); err != nil || string(b) != "important\n" {
		t.Fatalf("nested .pitr file not restored: %q err=%v", b, err)
	}
	if err := Verify(dir, st, sp.Step); err != nil {
		t.Fatalf("Verify after restoring the nested file: %v", err)
	}
	// The workspace's own state directory stays out of the manifest.
	if _, ok := m.Files[".pitr/state.db"]; ok {
		t.Fatalf("root state directory leaked into the manifest: %v", m.Files)
	}
}
