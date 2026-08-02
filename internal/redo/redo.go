// Package redo writes and reads the append-only .pitr/redo.wal redo log.
//
// Between savepoints, the foreground watcher appends one entry per file-write
// event (path + new-content sha256 + size + op + timestamp). This is the
// database-redo-log analogue: a per-write transaction log over the working
// set. v0.1 rewinds to savepoint boundaries; sub-savepoint replay of this log
// is on the roadmap (§5 of the plan) and the log is recorded now so the
// primitive is complete.
package redo

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// Op is the kind of filesystem event recorded.
type Op string

const (
	OpWrite  Op = "write"
	OpCreate Op = "create"
	OpRemove Op = "remove"
	OpRename Op = "rename"
)

// Entry is a single redo-log line.
type Entry struct {
	Path   string `json:"path"` // relpath within the working set
	Sha256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Op     Op     `json:"op"`
	IsoTs  string `json:"iso_ts"`
}

// NewEntry returns an Entry with the timestamp filled in.
func NewEntry(op Op, rel string, sha string, size int64) Entry {
	return Entry{Op: op, Path: rel, Sha256: sha, Size: size, IsoTs: time.Now().UTC().Format(time.RFC3339Nano)}
}

// Append writes one JSON line to the redo log.
func Append(st *store.Store, e Entry) error {
	if e.IsoTs == "" {
		e.IsoTs = time.Now().UTC().Format(time.RFC3339Nano)
	}
	f, err := os.OpenFile(st.RedoLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open redo log: %w", err)
	}
	defer f.Close()
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode redo entry: %w", err)
	}
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write redo log: %w", err)
	}
	return nil
}

// Read returns every redo entry in append order.
func Read(st *store.Store) ([]Entry, error) {
	f, err := os.Open(st.RedoLogPath())
	if err != nil {
		return nil, fmt.Errorf("open redo log: %w", err)
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("parse redo entry: %w", err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// Truncate empties the redo log. Called after a savepoint so the log only
// holds writes that happened since the last savepoint boundary.
func Truncate(st *store.Store) error {
	if err := os.Truncate(st.RedoLogPath(), 0); err != nil {
		return fmt.Errorf("truncate redo log: %w", err)
	}
	return nil
}
