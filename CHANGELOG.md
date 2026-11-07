# Changelog

All notable changes to pitrrwnd are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-11-08

### Fixed
- `pitr savepoint` on a `--root` reached through a symlink no longer "succeeds"
  with an empty working set. The root was walked without being followed, so a
  linked workspace directory produced a manifest with zero files
  (`working_set_sha256` was the hash of the empty string) and `pitr verify`
  agreed; the watcher was equally silent on the same root. `--root` is now
  resolved to its real directory once, so every command and the watcher agree on
  one working-set identity, and a non-directory root fails instead of
  snapshotting nothing.
- A nested directory named `.pitr` is no longer mistaken for the workspace's
  state directory. Only `<root>/.pitr` is state; previously any directory with
  that name was hidden from `pitr savepoint`, left unrestorable by
  `pitr rewind`, and ignored by `pitr verify` — which could report OK for a
  working set that had lost a file it had snapshotted.
- `pitr rewind` validates the snapshot against the manifest before deleting or
  overwriting any working-set file. A missing or corrupt snapshot file now fails
  with the working set untouched instead of leaving it half-rewound, and the
  documented rewind-then-verify byte-equality guarantee is checked rather than
  assumed.
- A failed `pitr verify` records why it failed in the audit ledger
  (`"result": "fail: <reason>"`) instead of a bare `"fail"`, so the local
  compliance record keeps the divergence the CLI prints.

### Changed
- All version surfaces bumped to 0.4.0 in lockstep: `VERSION` file, CLI
  `--version`, the on-disk `store.Version` reported in the audit-export bundle
  manifest, the CHANGELOG release heading, and the site's content version /
  footer tag. The lockstep test now pins the CHANGELOG head and both site
  surfaces in addition to the Go version surfaces.

## [0.3.0] - 2026-10-10

### Fixed
- `pitr watch` no longer locks out every concurrent command. The watcher
  session held the exclusive bbolt lock on `.pitr/state.db` for its entire
  lifetime, so running `pitr savepoint`, `pitr rewind`, `pitr verify`, `pitr
  log` or `pitr audit-export` in another terminal failed after ~3s with
  `open state db: timeout` — exactly the documented workflow (watch in the
  foreground while stateless commands run alongside). The watcher now takes
  only the redo-log path; the state DB handle is released before it blocks.

### Added
- `pitr log` now shows a "Redo log (since last savepoint)" section listing the
  pending per-write events recorded by the watcher. The redo log was
  previously only visible inside the `audit-export` bundle as raw JSONL. A
  torn or unreadable redo log (possible now that the watcher appends while a
  concurrent savepoint truncates) is reported as a warning instead of failing
  the command; the audit ledger keeps its strict integrity contract.

### Changed
- All version surfaces bumped to 0.3.0 in lockstep: `VERSION` file, CLI
  `--version`, the on-disk `store.Version` reported in the audit-export bundle
  manifest, and the site's content version / footer tag. A new lockstep test
  pins the CLI default, the VERSION file and `store.Version` together so they
  cannot drift again.

## [0.2.0] - 2026-09-09

### Fixed
- Savepoint manifests now hash the snapshot bytes instead of re-reading the
  working-set source after the copy. Previously a file rewritten while
  `pitr savepoint` was running (exactly the foreground-watch workflow) could
  produce a manifest that no longer described the snapshot, so
  `pitr rewind --step N && pitr verify --step N` exited non-zero on a
  byte-level restore. The manifest is now internally consistent with the
  snapshot by construction.
- `pitr savepoint`, `pitr rewind` and `pitr verify` no longer silently
  swallow audit-ledger and redo-log truncation failures. A missing or
  unwritable `.pitr/audit.jsonl` now exits non-zero instead of recording an
  unaudited savepoint or restore — the rewind error states that the working
  set was already restored and only the ledger append failed.

### Changed
- All version surfaces bumped to 0.2.0 in lockstep: `VERSION` file, CLI
  `--version`, the on-disk `store.Version` reported in the audit-export
  bundle manifest, and the site's content version / footer tag.

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

[Unreleased]: https://github.com/SuperMarioYL/pitrrwnd/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/SuperMarioYL/pitrrwnd/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/SuperMarioYL/pitrrwnd/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/SuperMarioYL/pitrrwnd/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/SuperMarioYL/pitrrwnd/releases/tag/v0.1.0
