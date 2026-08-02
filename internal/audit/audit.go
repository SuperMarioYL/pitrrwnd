// Package audit writes and reads the append-only .pitr/audit.jsonl ledger.
//
// Every meaningful operation (init, savepoint, rewind, verify) appends a line
// with the operation, step, label, working-set sha256, an ISO-8601 timestamp,
// hostname, and user. This is the 等保 "可审计地还原至介入前状态" record,
// generated locally and never leaving the box.
package audit

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"time"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// Op is the kind of auditable operation.
type Op string

const (
	OpInit      Op = "init"
	OpSavepoint Op = "savepoint"
	OpRewind    Op = "rewind"
	OpVerify    Op = "verify"
)

// Entry is a single audit-ledger line. Lines are JSON objects separated by
// newlines (JSONL).
type Entry struct {
	Op               Op     `json:"op"`
	Step             int    `json:"step,omitempty"`
	Label            string `json:"label,omitempty"`
	WorkingSetSha256 string `json:"working_set_sha256,omitempty"`
	IsoTs            string `json:"iso_ts"`
	Hostname         string `json:"hostname"`
	User             string `json:"user"`
	Result           string `json:"result,omitempty"` // "ok" or "fail:<reason>"
}

// NewEntry returns an Entry with op, timestamp, hostname, and user filled in.
func NewEntry(op Op) Entry {
	host, _ := os.Hostname()
	return Entry{
		Op:       op,
		IsoTs:    time.Now().UTC().Format(time.RFC3339Nano),
		Hostname: host,
		User:     currentUser(),
	}
}

// Append writes one JSON line to the audit ledger. It fills timestamp,
// hostname, and user if the caller left them blank.
func Append(st *store.Store, e Entry) error {
	if e.IsoTs == "" {
		e.IsoTs = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if e.Hostname == "" {
		e.Hostname, _ = os.Hostname()
	}
	if e.User == "" {
		e.User = currentUser()
	}
	f, err := os.OpenFile(st.AuditPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open audit ledger: %w", err)
	}
	defer f.Close()
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode audit entry: %w", err)
	}
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write audit ledger: %w", err)
	}
	return nil
}

// Read returns every audit entry in append order. A malformed line is an
// integrity error and is reported rather than silently skipped.
func Read(st *store.Store) ([]Entry, error) {
	f, err := os.Open(st.AuditPath())
	if err != nil {
		return nil, fmt.Errorf("open audit ledger: %w", err)
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("audit ledger line %d: %w", lineNo, err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read audit ledger: %w", err)
	}
	return out, nil
}

// Hash returns the sha256 of the raw audit ledger bytes, for the 等保 bundle
// manifest.
func Hash(st *store.Store) (string, error) {
	data, err := os.ReadFile(st.AuditPath())
	if err != nil {
		return "", err
	}
	return sha256Hex(data), nil
}

func currentUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	if v := os.Getenv("LOGNAME"); v != "" {
		return v
	}
	return "unknown"
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
