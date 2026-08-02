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
