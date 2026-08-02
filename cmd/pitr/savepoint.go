package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/SuperMarioYL/pitrrwnd/internal/audit"
	"github.com/SuperMarioYL/pitrrwnd/internal/redo"
	"github.com/SuperMarioYL/pitrrwnd/internal/snapshot"
)

var savepointCmd = &cobra.Command{
	Use:   "savepoint",
	Short: "Write a COW snapshot + sha256 manifest at a step boundary",
	Long: `Writes a reflink/copy COW snapshot of the working set plus a sha256
manifest of every file, and records the savepoint in the audit ledger. The
redo log is truncated at the savepoint boundary so it holds only writes since
this savepoint.`,
	RunE: runSavepoint,
}

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify the working set is byte-level equal to a savepoint",
	Long: `Hashes every working-set file and exits 0 iff every sha256 matches the
savepoint manifest and no files were added or removed. The killer demo:
after ` + "`pitr rewind --step N`" + `, ` + "`pitr verify --step N`" + ` exits 0 byte-equal.`,
	RunE: runVerify,
}

var (
	savepointLabel string
	verifyStep     int
)

func init() {
	savepointCmd.Flags().StringVarP(&savepointLabel, "label", "l", "", "human description of this savepoint")
	rootCmd.AddCommand(savepointCmd)

	verifyCmd.Flags().IntVarP(&verifyStep, "step", "s", 0, "savepoint step to verify against (required)")
	_ = verifyCmd.MarkFlagRequired("step")
	rootCmd.AddCommand(verifyCmd)
}

func runSavepoint(_ *cobra.Command, _ []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	sp, err := snapshot.Take(workspaceRoot, st, savepointLabel, currentActor())
	if err != nil {
		return fmt.Errorf("take savepoint: %w", err)
	}

	e := audit.NewEntry(audit.OpSavepoint)
	e.Step = sp.Step
	e.Label = sp.Label
	e.WorkingSetSha256 = sp.WorkingSetSha256
	e.Result = "ok"
	_ = audit.Append(st, e)

	// Truncate the redo log at the savepoint boundary.
	_ = redo.Truncate(st)

	fmt.Printf("savepoint step=%d label=%q working_set_sha256=%s\n", sp.Step, sp.Label, sp.WorkingSetSha256)
	return nil
}

func runVerify(_ *cobra.Command, _ []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	verr := snapshot.Verify(workspaceRoot, st, verifyStep)
	e := audit.NewEntry(audit.OpVerify)
	e.Step = verifyStep
	if verr != nil {
		e.Result = "fail"
	} else {
		e.Result = "ok"
	}
	_ = audit.Append(st, e)

	if verr != nil {
		fmt.Printf("verify step %d: FAIL — %v\n", verifyStep, verr)
		return verr // non-zero exit
	}
	fmt.Printf("verify step %d: OK (working set sha256 matches manifest)\n", verifyStep)
	return nil
}
