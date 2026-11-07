package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/SuperMarioYL/pitrrwnd/internal/store"
)

// TestVersionSurfacesLockstep pins every surface that states the product
// version together: the CLI default (overridable at release time via
// -ldflags -X main.version=...), the VERSION file (what Makefile/goreleaser
// feed into those flags), the store's on-disk format constant (reported in
// audit-export bundle manifests), the newest released CHANGELOG heading, and the
// Pages site (meta.content_version plus both footer tags). Bumping any one
// surface alone fails here. Pure Go — no binary invocation — so it runs on any
// CI runner.
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

	// The newest released changelog heading must BE this version, not merely
	// contain it; a stale head is exactly the drift this guard exists for.
	changelog, err := os.ReadFile(filepath.Join("..", "..", "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read CHANGELOG: %v", err)
	}
	if got := newestReleaseHeading(string(changelog)); got != fileVersion {
		t.Fatalf("newest CHANGELOG release heading %q != VERSION file %q — add the release section in lockstep", got, fileVersion)
	}

	// The Pages site carries the product version in meta.content_version and in
	// both footer tags (the site build has no other version input).
	siteRaw, err := os.ReadFile(filepath.Join("..", "..", "web", "site.json"))
	if err != nil {
		t.Fatalf("read web/site.json: %v", err)
	}
	var site struct {
		Meta struct {
			ContentVersion string `json:"content_version"`
		} `json:"meta"`
		Footer struct {
			Tag struct {
				Zh string `json:"zh"`
				En string `json:"en"`
			} `json:"tag"`
		} `json:"footer"`
	}
	if err := json.Unmarshal(siteRaw, &site); err != nil {
		t.Fatalf("parse web/site.json: %v", err)
	}
	wantSiteVersion := "v" + fileVersion
	if site.Meta.ContentVersion != wantSiteVersion {
		t.Fatalf("web/site.json meta.content_version %q != %q — bump the site in lockstep", site.Meta.ContentVersion, wantSiteVersion)
	}
	for locale, tag := range map[string]string{"zh": site.Footer.Tag.Zh, "en": site.Footer.Tag.En} {
		if !strings.HasSuffix(tag, wantSiteVersion) {
			t.Fatalf("web/site.json footer.tag.%s %q does not end with %q — bump the site in lockstep", locale, tag, wantSiteVersion)
		}
	}
}

// releaseHeadingRE matches a released changelog heading such as `## [0.4.0] -`
// while skipping `## [Unreleased]`.
var releaseHeadingRE = regexp.MustCompile(`(?m)^## \[(\d+\.\d+\.\d+)\]`)

func newestReleaseHeading(changelog string) string {
	m := releaseHeadingRE.FindStringSubmatch(changelog)
	if m == nil {
		return ""
	}
	return m[1]
}
