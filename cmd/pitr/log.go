package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SuperMarioYL/pitrrwnd/internal/audit"
	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Print the savepoint timeline and audit trail",
	RunE:  runLog,
}

var auditExportCmd = &cobra.Command{
	Use:   "audit-export",
	Short: "Emit a 等保 reviewer bundle (audit.jsonl + sha256 manifest + version)",
	Long: `Writes a tar.gz bundle containing the audit ledger (audit.jsonl), the
savepoint index (savepoints.json), and a bundle manifest with the tool
version, workspace creation time, hostname, and the sha256 of the audit
ledger. Drop this bundle on a reviewer's desk; it is fully self-contained.`,
	RunE: runAuditExport,
}

var exportOut string

func init() {
	rootCmd.AddCommand(logCmd)

	auditExportCmd.Flags().StringVarP(&exportOut, "out", "o", "pitr-audit-bundle.tar.gz", "output bundle path")
	rootCmd.AddCommand(auditExportCmd)
}

func runLog(_ *cobra.Command, _ []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	sps, err := st.ListSavepoints()
	if err != nil {
		return fmt.Errorf("list savepoints: %w", err)
	}

	fmt.Println("== Savepoints ==")
	if len(sps) == 0 {
		fmt.Println("  (none yet — run `pitr savepoint --label \"<step>\"`)")
	}
	for _, sp := range sps {
		fmt.Printf("  step %-4d  %s\n", sp.Step, formatSavepoint(sp))
	}

	entries, err := audit.Read(st)
	if err != nil {
		return fmt.Errorf("read audit ledger: %w", err)
	}
	fmt.Println("\n== Audit trail ==")
	if len(entries) == 0 {
		fmt.Println("  (no entries)")
	}
	for _, e := range entries {
		fmt.Printf("  %s  %-9s %s\n", e.IsoTs, string(e.Op), formatAudit(e))
	}
	return nil
}

func formatSavepoint(sp store.Savepoint) string {
	label := sp.Label
	if label == "" {
		label = "-"
	}
	return fmt.Sprintf("label=%-24q actor=%-12s created=%s\n             working_set_sha256=%s",
		label, sp.Actor, sp.CreatedAt.Format(time.RFC3339), sp.WorkingSetSha256)
}

func formatAudit(e audit.Entry) string {
	var parts []string
	if e.Step != 0 {
		parts = append(parts, fmt.Sprintf("step=%d", e.Step))
	}
	if e.Label != "" {
		parts = append(parts, fmt.Sprintf("label=%q", e.Label))
	}
	if e.WorkingSetSha256 != "" {
		parts = append(parts, fmt.Sprintf("ws_sha256=%s", e.WorkingSetSha256))
	}
	parts = append(parts, fmt.Sprintf("user=%s@%s", e.User, e.Hostname))
	if e.Result != "" {
		parts = append(parts, fmt.Sprintf("result=%s", e.Result))
	}
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}

// bundleManifest is the JSON manifest placed at the root of the audit bundle.
type bundleManifest struct {
	Tool            string    `json:"tool"`
	Version         string    `json:"version"`
	WorkspaceRoot   string    `json:"workspace_root"`
	WorkspaceCreatedAt string `json:"workspace_created_at"`
	Hostname        string    `json:"hostname"`
	GeneratedAt     time.Time `json:"generated_at"`
	AuditSha256     string    `json:"audit_sha256"`
	SavepointCount  int       `json:"savepoint_count"`
}

func runAuditExport(_ *cobra.Command, _ []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()

	auditHash, err := audit.Hash(st)
	if err != nil {
		return fmt.Errorf("hash audit ledger: %w", err)
	}
	sps, err := st.ListSavepoints()
	if err != nil {
		return fmt.Errorf("list savepoints: %w", err)
	}
	created, _ := st.MetaCreatedAt()
	hostname, _ := os.Hostname()
	root, _ := absRoot()

	manifest := bundleManifest{
		Tool:               "pitrrwnd",
		Version:            store.Version,
		WorkspaceRoot:      root,
		WorkspaceCreatedAt: created,
		Hostname:           hostname,
		GeneratedAt:        time.Now().UTC(),
		AuditSha256:        auditHash,
		SavepointCount:     len(sps),
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	savepointBytes, err := json.MarshalIndent(sps, "", "  ")
	if err != nil {
		return err
	}

	f, err := os.Create(exportOut)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	if err := addTarFile(tw, "manifest.json", manifestBytes); err != nil {
		return err
	}
	if err := addTarFile(tw, "savepoints.json", savepointBytes); err != nil {
		return err
	}
	auditData, err := os.ReadFile(st.AuditPath())
	if err != nil {
		return fmt.Errorf("read audit ledger: %w", err)
	}
	if err := addTarFile(tw, "audit.jsonl", auditData); err != nil {
		return err
	}
	redoData, err := os.ReadFile(st.RedoLogPath())
	if err != nil {
		return fmt.Errorf("read redo log: %w", err)
	}
	if err := addTarFile(tw, "redo.wal", redoData); err != nil {
		return err
	}

	fmt.Printf("audit bundle written: %s\n", exportOut)
	fmt.Printf("  audit_sha256=%s\n", auditHash)
	fmt.Printf("  savepoints=%d\n", len(sps))
	fmt.Printf("  generated_at=%s\n", manifest.GeneratedAt.Format(time.RFC3339))
	return nil
}

func addTarFile(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name: name,
		Mode: 0o644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write tar header %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write tar body %s: %w", name, err)
	}
	return nil
}
