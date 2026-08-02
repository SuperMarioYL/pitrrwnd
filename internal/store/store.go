// Package store manages the .pitr/ state directory for a pitrrwnd workspace.
//
// The store is the single source of truth for on-disk layout:
//
//	<root>/.pitr/
//	  ├── state.db            bbolt DB: savepoint index + metadata
//	  ├── snapshots/<step>/   COW copy of the working set at savepoint <step>
//	  ├── manifests/<step>.json  sha256 manifest of the working set at <step>
//	  ├── redo.wal             append-only redo log (file-write events)
//	  └── audit.jsonl         append-only audit ledger
//
// All paths are derived from Store so the rest of the codebase never builds
// ".pitr/..." strings by hand.
package store

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Version is the on-disk state format version, persisted on init.
const Version = "0.1.0"

const (
	// PitrDirName is the name of the state directory inside a workspace root.
	PitrDirName = ".pitr"

	dbFileName       = "state.db"
	snapshotsDirName = "snapshots"
	manifestsDirName = "manifests"
	redoLogFileName  = "redo.wal"
	auditFileName    = "audit.jsonl"

	bucketSavepoints = "savepoints" // 8-byte big-endian step -> JSON Savepoint
	bucketMeta       = "meta"

	keyNextStep  = "next_step"
	keyCreatedAt = "created_at"
	keyVersion   = "version"
)

// ErrNotInitialized is returned when a workspace has no .pitr/ state directory.
var ErrNotInitialized = errors.New("pitrrwnd not initialized: run `pitr init` first")

// Savepoint is a savepoint index record persisted in the state DB. The full
// per-file manifest lives in a sibling JSON file (see ManifestPath).
type Savepoint struct {
	Step             int       `json:"step"`
	Label            string    `json:"label"`
	Actor            string    `json:"actor"`
	CreatedAt        time.Time `json:"created_at"`
	WorkingSetSha256 string    `json:"working_set_sha256"`
}

// Store is the on-disk .pitr/ state manager for a workspace root.
type Store struct {
	Root    string
	PitrDir string
	db      *bolt.DB
}

// Init creates the .pitr/ state directory and a fresh state DB for root.
// It is idempotent: re-running on an existing workspace is a no-op aside from
// re-recording metadata.
func Init(root, actor string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	pitr := filepath.Join(abs, PitrDirName)
	for _, sub := range []string{pitr, filepath.Join(pitr, snapshotsDirName), filepath.Join(pitr, manifestsDirName)} {
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", sub, err)
		}
	}
	// Touch the append-only files so subsequent appends are pure writes.
	for _, name := range []string{filepath.Join(pitr, redoLogFileName), filepath.Join(pitr, auditFileName)} {
		f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("create %s: %w", name, err)
		}
		_ = f.Close()
	}
	st := &Store{Root: abs, PitrDir: pitr}
	db, err := bolt.Open(st.DBPath(), 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open state db: %w", err)
	}
	st.db = db
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(bucketSavepoints)); err != nil {
			return err
		}
		meta, err := tx.CreateBucketIfNotExists([]byte(bucketMeta))
		if err != nil {
			return err
		}
		if meta.Get([]byte(keyCreatedAt)) == nil {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if err := meta.Put([]byte(keyCreatedAt), []byte(now)); err != nil {
				return err
			}
			if err := meta.Put([]byte(keyVersion), []byte(Version)); err != nil {
				return err
			}
			if err := meta.Put([]byte(keyNextStep), encodeStep(1)); err != nil {
				return err
			}
			if err := meta.Put([]byte("init_actor"), []byte(actor)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init state db: %w", err)
	}
	return st, nil
}

// Open opens an existing workspace's state. Returns ErrNotInitialized if the
// workspace has not been initialized.
func Open(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	pitr := filepath.Join(abs, PitrDirName)
	if fi, err := os.Stat(pitr); err != nil || !fi.IsDir() {
		return nil, ErrNotInitialized
	}
	st := &Store{Root: abs, PitrDir: pitr}
	db, err := bolt.Open(st.DBPath(), 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open state db: %w", err)
	}
	st.db = db
	// Ensure required buckets exist (defensive against a half-built state).
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(bucketSavepoints)); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists([]byte(bucketMeta))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure buckets: %w", err)
	}
	return st, nil
}

// Close releases the state DB handle.
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DBPath returns the path to the bbolt state DB.
func (s *Store) DBPath() string { return filepath.Join(s.PitrDir, dbFileName) }

// SnapshotsDir returns the snapshots root.
func (s *Store) SnapshotsDir() string { return filepath.Join(s.PitrDir, snapshotsDirName) }

// SnapshotDir returns the snapshot root for step n (whether or not it exists).
func (s *Store) SnapshotDir(step int) string {
	return filepath.Join(s.SnapshotsDir(), fmt.Sprintf("%010d", step))
}

// ManifestPath returns the manifest JSON path for step n.
func (s *Store) ManifestPath(step int) string {
	return filepath.Join(s.PitrDir, manifestsDirName, fmt.Sprintf("%010d.json", step))
}

// RedoLogPath returns the redo-log file path.
func (s *Store) RedoLogPath() string { return filepath.Join(s.PitrDir, redoLogFileName) }

// AuditPath returns the audit-ledger file path.
func (s *Store) AuditPath() string { return filepath.Join(s.PitrDir, auditFileName) }

// AllocStep atomically reserves and returns the next savepoint step number.
func (s *Store) AllocStep() (int, error) {
	var step int
	err := s.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return errors.New("meta bucket missing")
		}
		raw := meta.Get([]byte(keyNextStep))
		if raw == nil {
			step = 1
		} else {
			step = int(decodeStep(raw))
		}
		return meta.Put([]byte(keyNextStep), encodeStep(step+1))
	})
	return step, err
}

// SaveSavepoint persists a savepoint index record.
func (s *Store) SaveSavepoint(sp Savepoint) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSavepoints))
		if b == nil {
			return errors.New("savepoints bucket missing")
		}
		data, err := json.Marshal(sp)
		if err != nil {
			return err
		}
		return b.Put(encodeStep(sp.Step), data)
	})
}

// GetSavepoint loads a savepoint index record by step.
func (s *Store) GetSavepoint(step int) (Savepoint, error) {
	var sp Savepoint
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSavepoints))
		if b == nil {
			return errors.New("savepoints bucket missing")
		}
		raw := b.Get(encodeStep(step))
		if raw == nil {
			return fmt.Errorf("no savepoint at step %d", step)
		}
		return json.Unmarshal(raw, &sp)
	})
	return sp, err
}

// ListSavepoints returns all savepoints in ascending step order.
func (s *Store) ListSavepoints() ([]Savepoint, error) {
	var out []Savepoint
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSavepoints))
		if b == nil {
			return errors.New("savepoints bucket missing")
		}
		return b.ForEach(func(_, v []byte) error {
			var sp Savepoint
			if err := json.Unmarshal(v, &sp); err != nil {
				return err
			}
			out = append(out, sp)
			return nil
		})
	})
	return out, err
}

// MetaCreatedAt returns the workspace init timestamp, if recorded.
func (s *Store) MetaCreatedAt() (string, error) {
	var v string
	err := s.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte(bucketMeta))
		if meta == nil {
			return nil
		}
		if raw := meta.Get([]byte(keyCreatedAt)); raw != nil {
			v = string(raw)
		}
		return nil
	})
	return v, err
}

// encodeStep encodes a step as an 8-byte big-endian int64 so bbolt's
// byte-sorted iteration yields ascending numeric order.
func encodeStep(step int) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(step))
	return b[:]
}

func decodeStep(b []byte) int64 {
	if len(b) < 8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(b[:8]))
}
