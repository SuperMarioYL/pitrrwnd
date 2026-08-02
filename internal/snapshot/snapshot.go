// Package snapshot implements the file system PITR primitive: per-step COW
// savepoints with sha256 manifests, byte-level verification, and rewind.
//
// A savepoint is an independent copy of the working set (a reflink clone on
// filesystems that support FICLONE — XFS/Btrfs — or a full copy fallback on
// ext4/APFS) plus a sha256 manifest of every file. Restore copies the snapshot
// back over the working set so that a subsequent Verify exits 0 byte-equal.
package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// Manifest is the per-step file manifest persisted as JSON next to the snapshot.
type Manifest struct {
	Step             int               `json:"step"`
	Label            string            `json:"label"`
	CreatedAt        time.Time         `json:"created_at"`
	Actor            string            `json:"actor"`
	WorkingSetSha256 string            `json:"working_set_sha256"`
	Files            map[string]string `json:"files"` // relpath -> sha256 hex
}

// errUnsupported is returned by reflink on platforms without a clone ioctl.
var errUnsupported = errors.New("reflink clone unsupported on this platform")

// Take writes a COW snapshot + sha256 manifest for the working set at root and
// records the savepoint in the index. Returns the savepoint summary.
func Take(root string, st *store.Store, label, actor string) (store.Savepoint, error) {
	step, err := st.AllocStep()
	if err != nil {
		return store.Savepoint{}, fmt.Errorf("alloc step: %w", err)
	}
	snapDir := st.SnapshotDir(step)
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return store.Savepoint{}, fmt.Errorf("create snapshot dir: %w", err)
	}

	files, err := walkFiles(root)
	if err != nil {
		return store.Savepoint{}, fmt.Errorf("walk working set: %w", err)
	}

	manifest := Manifest{
		Step:      step,
		Label:     label,
		Actor:     actor,
		CreatedAt: time.Now().UTC(),
		Files:     make(map[string]string, len(files)),
	}
	for _, rel := range files {
		src := filepath.Join(root, rel)
		dst := filepath.Join(snapDir, rel)
		if err := cloneOrCopy(src, dst); err != nil {
			return store.Savepoint{}, fmt.Errorf("snapshot %s: %w", rel, err)
		}
		h, err := hashFile(src)
		if err != nil {
			return store.Savepoint{}, fmt.Errorf("hash %s: %w", rel, err)
		}
		manifest.Files[rel] = h
	}
	manifest.WorkingSetSha256 = workingSetDigest(manifest.Files)

	if err := writeManifest(st.ManifestPath(step), manifest); err != nil {
		return store.Savepoint{}, fmt.Errorf("write manifest: %w", err)
	}

	sp := store.Savepoint{
		Step:             step,
		Label:            label,
		Actor:            actor,
		CreatedAt:        manifest.CreatedAt,
		WorkingSetSha256: manifest.WorkingSetSha256,
	}
	if err := st.SaveSavepoint(sp); err != nil {
		return store.Savepoint{}, fmt.Errorf("save savepoint: %w", err)
	}
	return sp, nil
}

// Verify hashes every working-set file and returns nil iff they all match the
// manifest for step (and no extra files exist). A non-nil error describes the
// first divergence; the command layer maps it to a non-zero exit code.
func Verify(root string, st *store.Store, step int) error {
	manifest, err := LoadManifest(st, step)
	if err != nil {
		return err
	}
	for rel, want := range manifest.Files {
		h, err := hashFile(filepath.Join(root, rel))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("verify: missing file %s", rel)
			}
			return fmt.Errorf("verify %s: %w", rel, err)
		}
		if h != want {
			return fmt.Errorf("verify: %s sha256 mismatch (want %s, got %s)", rel, want, h)
		}
	}
	// Detect files added to the working set after the savepoint.
	files, err := walkFiles(root)
	if err != nil {
		return fmt.Errorf("walk working set: %w", err)
	}
	for _, rel := range files {
		if _, ok := manifest.Files[rel]; !ok {
			return fmt.Errorf("verify: extra file not in savepoint %d: %s", step, rel)
		}
	}
	return nil
}

// Restore rewinds the working set to step by copying snapshot files back and
// removing any working-set file not present at the savepoint. Returns the
// savepoint summary (for the audit ledger).
func Restore(root string, st *store.Store, step int) (store.Savepoint, error) {
	manifest, err := LoadManifest(st, step)
	if err != nil {
		return store.Savepoint{}, err
	}
	snapDir := st.SnapshotDir(step)
	for rel := range manifest.Files {
		src := filepath.Join(snapDir, rel)
		dst := filepath.Join(root, rel)
		// Remove the working-set file first so an in-place hardlink (if any
		// earlier copy left one) is broken and we write a clean independent file.
		if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
			return store.Savepoint{}, fmt.Errorf("remove %s: %w", rel, err)
		}
		if err := cloneOrCopy(src, dst); err != nil {
			return store.Savepoint{}, fmt.Errorf("restore %s: %w", rel, err)
		}
	}
	// Remove files the agent created after the savepoint.
	files, err := walkFiles(root)
	if err != nil {
		return store.Savepoint{}, fmt.Errorf("walk working set: %w", err)
	}
	for _, rel := range files {
		if _, ok := manifest.Files[rel]; !ok {
			if err := os.Remove(filepath.Join(root, rel)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return store.Savepoint{}, fmt.Errorf("remove extra %s: %w", rel, err)
			}
		}
	}
	return store.Savepoint{
		Step:             manifest.Step,
		Label:            manifest.Label,
		Actor:            manifest.Actor,
		CreatedAt:        manifest.CreatedAt,
		WorkingSetSha256: manifest.WorkingSetSha256,
	}, nil
}

// LoadManifest reads the per-step manifest JSON.
func LoadManifest(st *store.Store, step int) (Manifest, error) {
	data, err := os.ReadFile(st.ManifestPath(step))
	if err != nil {
		return Manifest{}, fmt.Errorf("load manifest step %d: %w", step, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse manifest step %d: %w", step, err)
	}
	return m, nil
}

func writeManifest(path string, m Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// cloneOrCopy makes dst share data with src via a reflink clone when the
// filesystem supports it, falling back to a full copy otherwise. The full-copy
// fallback is what makes snapshots safe against in-place agent edits on
// filesystems (ext4, APFS) without reflink support.
func cloneOrCopy(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := reflink(src, dst); err == nil {
		return preserveMode(src, dst)
	}
	return copyFile(src, dst)
}

func preserveMode(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode())
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// workingSetDigest returns a deterministic sha256 over the (relpath, sha256)
// pairs, sorted by relpath. This is the single digest for the whole working
// set at a savepoint and is what the audit ledger records.
func workingSetDigest(files map[string]string) string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s\x00%s\n", k, files[k])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// walkFiles returns the relative paths of all regular files under root,
// skipping the .pitr/ state directory and non-regular entries (symlinks,
// devices). Paths use forward slashes for cross-platform manifest stability.
func walkFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			if p == root {
				return nil
			}
			if filepath.Base(p) == store.PitrDirName {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}
