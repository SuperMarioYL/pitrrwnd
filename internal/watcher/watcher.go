// Package watcher runs a foreground fsnotify watcher over the working set,
// appending a redo-log entry per file event. This is the agent-session
// transaction log: it runs while the agent works, then savepoint/rewind are
// stateless commands operating on .pitr/.
package watcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"

	"github.com/SuperMarioYL/pitrrwnd/internal/redo"
	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// Watch runs the foreground watcher over root. It blocks until ctx is
// cancelled or the watcher fails. New directories created during the session
// are added to the watch tree so nested agent edits are captured too.
func Watch(ctx context.Context, root string, st *store.Store, onEvent func(redo.Entry)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer w.Close()
	if err := addRecursive(w, root); err != nil {
		return fmt.Errorf("seed watch tree: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			if err != nil {
				return fmt.Errorf("watcher: %w", err)
			}
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if e := handle(st, root, w, ev); e.Path != "" && onEvent != nil {
				onEvent(e)
			}
		}
	}
}

// handle processes a single fsnotify event and returns the redo entry it
// appended (zero value Entry if the event was skipped, e.g. inside .pitr).
func handle(st *store.Store, root string, w *fsnotify.Watcher, ev fsnotify.Event) redo.Entry {
	rel, err := filepath.Rel(root, ev.Name)
	if err != nil {
		return redo.Entry{}
	}
	// Never observe the state directory itself.
	if rel == store.PitrDirName || filepath.HasPrefix(rel, store.PitrDirName+string(filepath.Separator)) {
		return redo.Entry{}
	}
	// New directory: fold it into the watch tree.
	if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
		if ev.Has(fsnotify.Create) {
			_ = w.Add(ev.Name)
		}
		return redo.Entry{}
	}
	op := redo.OpWrite
	switch {
	case ev.Has(fsnotify.Create):
		op = redo.OpCreate
	case ev.Has(fsnotify.Remove):
		op = redo.OpRemove
	case ev.Has(fsnotify.Rename):
		op = redo.OpRename
	}
	var sha string
	var size int64 = -1
	if op != redo.OpRemove {
		if h, s, err := hashAndSize(ev.Name); err == nil {
			sha, size = h, s
		}
	}
	e := redo.NewEntry(op, filepath.ToSlash(rel), sha, size)
	_ = redo.Append(st, e)
	return e
}

// addRecursive adds every directory under root to the watcher, skipping .pitr.
func addRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if p != root && filepath.Base(p) == store.PitrDirName {
			return filepath.SkipDir
		}
		return w.Add(p)
	})
}

func hashAndSize(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), info.Size(), nil
}
