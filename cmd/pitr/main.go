// Command pitr is the pitrrwnd CLI: filesystem PITR (point-in-time-recovery)
// for agent workspaces. It turns a directory into a transactional workspace
// with per-step COW savepoints, a redo log, byte-level verify, and an
// append-only audit ledger — all local, never leaving the box.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// version is overridden at release time via -ldflags "-X main.version=<tag>".
var version = "0.4.0"

// workspaceRoot is the agent working set; defaults to the current directory.
var workspaceRoot string

var rootCmd = &cobra.Command{
	Use:   "pitr",
	Short: "pitrrwnd — filesystem PITR for agent workspaces",
	Long: `pitrrwnd (pitr) — filesystem point-in-time-recovery for agent workspaces.

Drop the pitr binary into an agent workspace, run ` + "`pitr init`" + `, then mark
savepoints at step boundaries. When the agent breaks something, ` + "`pitr rewind --step N`" + `
restores the working set byte-for-byte and ` + "`pitr verify --step N`" + ` exits 0 only
on a sha256 match. Every savepoint and rewind appends to an append-only audit
ledger (.pitr/audit.jsonl) with sha256 + ISO timestamp + hostname + user.

All state is local to .pitr/; nothing leaves the box.
`,
	Version: version,
	// Keep error output clean: print the error, not the full usage block.
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&workspaceRoot, "root", "C", ".",
		"workspace root (default: current directory)")
	rootCmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		return resolveWorkspaceRoot()
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// currentActor returns the system user driving the operation, for the audit
// ledger.
func currentActor() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	for _, k := range []string{"USER", "LOGNAME"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "unknown"
}

// openStore opens an existing workspace's .pitr/ state.
func openStore() (*store.Store, error) {
	return store.Open(workspaceRoot)
}

// absRoot returns the absolute workspace root.
func absRoot() (string, error) {
	return filepath.Abs(workspaceRoot)
}

// resolveWorkspaceRoot normalizes --root to one absolute, symlink-resolved
// working-set identity. Every command needs it: filepath.Walk does not descend
// a symlinked root and would report an empty working set (a savepoint would
// "succeed" with nothing in it), while fsnotify reports real paths that only
// match a resolved root. A root that does not exist yet keeps its absolute form
// so `pitr init` can create it.
func resolveWorkspaceRoot() error {
	abs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			workspaceRoot = abs
			return nil
		}
		return fmt.Errorf("resolve root %s: %w", abs, err)
	}
	workspaceRoot = resolved
	return nil
}
