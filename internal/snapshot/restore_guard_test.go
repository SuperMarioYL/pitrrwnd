package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// TestRestoreRefusesUnusableSnapshot pins v0.4.0's rewind guard: Restore deletes
// and overwrites working-set files, so it must refuse to start when the
// snapshot cannot put every manifest file back. Before the guard a missing or
// corrupt snapshot file left the working set half-rewound — files deleted that
// existed nowhere else.
func TestRestoreRefusesUnusableSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage func(t *testing.T, snapFile string)
	}{
		{
			name: "missing snapshot file",
			damage: func(t *testing.T, snapFile string) {
				t.Helper()
				if err := os.Remove(snapFile); err != nil {
					t.Fatalf("remove snapshot file: %v", err)
				}
			},
		},
		{
			name: "corrupt snapshot file",
			damage: func(t *testing.T, snapFile string) {
				t.Helper()
				if err := os.WriteFile(snapFile, []byte("corrupt\n"), 0o644); err != nil {
					t.Fatalf("corrupt snapshot file: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			st, err := store.Init(dir, "tester")
			if err != nil {
				t.Fatalf("Init: %v", err)
			}
			defer st.Close()

			writeFile(t, dir, "a.txt", "A-original\n")
			writeFile(t, dir, "b.txt", "B-original\n")
			sp, err := Take(dir, st, "good", "tester")
			if err != nil {
				t.Fatalf("Take: %v", err)
			}

			tc.damage(t, filepath.Join(st.SnapshotDir(sp.Step), "b.txt"))
			// Break the working set like an agent edit before the rewind.
			if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A-broken\n"), 0o644); err != nil {
				t.Fatalf("break a.txt: %v", err)
			}

			if _, err := Restore(dir, st, sp.Step); err == nil {
				t.Fatalf("Restore must refuse a snapshot it cannot restore")
			} else if !strings.Contains(err.Error(), "left untouched") {
				t.Fatalf("error should state the working set was left untouched, got: %v", err)
			}

			// The working set is exactly what it was before the refused rewind.
			if b, err := os.ReadFile(filepath.Join(dir, "a.txt")); err != nil || string(b) != "A-broken\n" {
				t.Fatalf("a.txt changed during a refused rewind: %q err=%v", b, err)
			}
			if b, err := os.ReadFile(filepath.Join(dir, "b.txt")); err != nil || string(b) != "B-original\n" {
				t.Fatalf("b.txt must still exist unchanged: %q err=%v", b, err)
			}
		})
	}
}
