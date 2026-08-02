package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/SuperMarioYL/pitrrwnd/internal/audit"
	"github.com/SuperMarioYL/pitrrwnd/internal/redo"
	"github.com/SuperMarioYL/pitrrwnd/internal/snapshot"
)

var rewindCmd = &cobra.Command{
	Use:   "rewind",
	Short: "Restore the working set to a savepoint (rewind to step N)",
	Long: `Restores working-set files from .pitr/snapshots/<N>/ and removes any file
the agent created after that savepoint. Appends a rewind entry to the audit
ledger (op, step, label, working_set_sha256, ISO timestamp, hostname, user)
and truncates the redo log. The original working set is never deleted without
a savepoint to restore from.`,
	RunE: runRewind,
}

var rewindStep int

func init() {
	rewindCmd.Flags().IntVarP(&rewindStep, "step", "s", 0, "savepoint step to rewind to (required)")
	_ = rewindCmd.MarkFlagRequired("step")
	rootCmd.AddCommand(rewindCmd)
}

func runRewind(_ *cobra.Command, _ []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	sp, err := snapshot.Restore(workspaceRoot, st, rewindStep)
	if err != nil {
		return fmt.Errorf("rewind step %d: %w", rewindStep, err)
	}

	e := audit.NewEntry(audit.OpRewind)
	e.Step = sp.Step
	e.Label = sp.Label
	e.WorkingSetSha256 = sp.WorkingSetSha256
	e.Result = "ok"
	_ = audit.Append(st, e)
	_ = redo.Truncate(st)

	fmt.Printf("rewound to step %d (%q); working_set_sha256=%s\n", sp.Step, sp.Label, sp.WorkingSetSha256)
	fmt.Println("run `pitr verify --step <N>` to confirm byte-level equality.")
	return nil
}
