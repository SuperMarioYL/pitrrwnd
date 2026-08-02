package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/SuperMarioYL/pitrrwnd/internal/audit"
	"github.com/SuperMarioYL/pitrrwnd/internal/redo"
	"github.com/SuperMarioYL/pitrrwnd/internal/store"
	"github.com/SuperMarioYL/pitrrwnd/internal/watcher"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .pitr/ state directory in the workspace",
	Long: `Creates .pitr/{snapshots,manifests,state.db,redo.wal,audit.jsonl} in the
workspace root and records an init entry in the audit ledger. After init,
run ` + "`pitr watch`" + ` in a foreground terminal while the agent works, then call
` + "`pitr savepoint --label \"<step>\"`" + ` at step boundaries.`,
	RunE: runInit,
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Run the foreground redo-log watcher over the workspace",
	Long: `Starts an fsnotify watcher over the workspace tree (excluding .pitr/) and
appends a redo-log entry per file write/create/remove. The watcher is
foreground by design (airgap invariant: no daemon, no background service).
Stop with Ctrl-C.`,
	RunE: runWatch,
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(watchCmd)
}

func runInit(cmd *cobra.Command, _ []string) error {
	st, err := store.Init(workspaceRoot, currentActor())
	if err != nil {
		return err
	}
	defer st.Close()

	// Record the init in the append-only audit ledger.
	if err := audit.Append(st, audit.NewEntry(audit.OpInit)); err != nil {
		return fmt.Errorf("audit init: %w", err)
	}

	fmt.Printf("pitrrwnd initialized at %s\n", st.PitrDir)
	fmt.Println("next:")
	fmt.Println("  pitr watch                        # foreground redo-log watcher (run while the agent works)")
	fmt.Println("  pitr savepoint --label \"before\"   # mark a step-boundary savepoint")
	fmt.Println("  pitr rewind --step 1              # restore the working set to step 1")
	fmt.Println("  pitr verify --step 1              # exit 0 iff byte-level equal")
	return nil
}

func runWatch(_ *cobra.Command, _ []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	root, err := absRoot()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "watching %s for file events (Ctrl-C to stop)\n", root)
	return watcher.Watch(ctx, root, st, func(e redo.Entry) {
		sha := e.Sha256
		if len(sha) > 8 {
			sha = sha[:8]
		}
		if sha == "" {
			sha = "-"
		}
		fmt.Printf("%-7s %-8s %10dB  %s\n", string(e.Op), sha, e.Size, e.Path)
	})
}
