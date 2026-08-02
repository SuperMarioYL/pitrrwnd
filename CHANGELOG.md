# Changelog

All notable changes to pitrrwnd are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-08-02

### Added
- `pitr init` — creates the `.pitr/` state directory (bbolt index, snapshots/,
  manifests/, `redo.wal`, `audit.jsonl`) and records an init entry in the audit
  ledger.
- `pitr savepoint --label` — writes a reflink/COW snapshot of the working set
  plus a sha256 manifest of every file; truncates the redo log at the boundary.
- `pitr watch` — foreground fsnotify watcher that appends a redo-log entry per
  file write/create/remove/rename.
- `pitr rewind --step N` — restores the working set byte-for-byte from the
  savepoint snapshot and removes files the agent added after the savepoint.
- `pitr verify --step N` — exits 0 iff every working-set file sha256 matches
  the savepoint manifest and no files were added or removed.
- `pitr log` — prints the savepoint timeline and the full append-only audit
  trail.
- `pitr audit-export -o bundle.tar.gz` — emits a self-contained reviewer
  bundle (audit ledger + savepoint index + manifest with sha256 and version).
- Reflink clone on XFS/Btrfs via `FICLONE`, with a full-copy fallback on
  ext4/APFS so snapshots stay safe against in-place agent edits.
- Bilingual README (zh-CN primary + English sibling), animated dark/light
  hero + architecture SVGs, vhs-rendered demo gif, and goreleaser release CI.

[Unreleased]: https://github.com/SuperMarioYL/pitrrwnd/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/SuperMarioYL/pitrrwnd/releases/tag/v0.1.0
