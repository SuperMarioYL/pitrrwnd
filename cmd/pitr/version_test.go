package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// TestVersionSurfacesLockstep pins the three version surfaces together: the
// CLI default (overridable at release time via -ldflags -X main.version=...),
// the VERSION file (what Makefile/goreleaser feed into those flags), and the
// store's on-disk format constant (reported in audit-export bundle manifests).
// Pure Go — no binary invocation — so it runs on any CI runner.
func TestVersionSurfacesLockstep(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	fileVersion := strings.TrimSpace(string(raw))

	if version != fileVersion {
		t.Fatalf("cmd/pitr version %q != VERSION file %q — bump them in lockstep", version, fileVersion)
	}
	if store.Version != fileVersion {
		t.Fatalf("store.Version %q != VERSION file %q — bump them in lockstep", store.Version, fileVersion)
	}
}
