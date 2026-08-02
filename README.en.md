<div align="right"><sub><b>English</b>&nbsp;&nbsp;⇄&nbsp;&nbsp;<a href="./README.md">简体中文</a></sub></div>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./assets/hero-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="./assets/hero-light.svg">
  <img src="./assets/hero-light.svg" width="880" alt="pitrrwnd — filesystem PITR for agent workspaces">
</picture>

<p align="center"><sub>On an airgapped 信创 box, after an agent corrupts files, pitrrwnd maps database PITR onto the working set — per-step COW savepoints plus a redo log rewind the working set byte-level to any step before the agent touched it. Fully local, with an audit trail fit for 等保 review; nothing ever leaves the box.</sub></p>

<p align="center">
  <a href="./LICENSE"><img src="https://img.shields.io/github/license/SuperMarioYL/pitrrwnd?color=blue" alt="license"></a>
  <a href="https://github.com/SuperMarioYL/pitrrwnd/releases"><img src="https://img.shields.io/github/v/release/SuperMarioYL/pitrrwnd" alt="release"></a>
  <a href="https://github.com/SuperMarioYL/pitrrwnd/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/SuperMarioYL/pitrrwnd/ci.yml?branch=main&label=ci" alt="ci"></a>
  <img src="https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white" alt="go">
  <img src="https://img.shields.io/badge/Coding%20Agent-ready-5E5CE6" alt="Coding Agent">
  <img src="https://img.shields.io/badge/Agent-workspace-5E5CE6" alt="Agent">
</p>

---

**The agent ran one bad 3am command and corrupted config and runtime state git can't see — `pitr rewind --step N` restores the working set to before that command in seconds, and `pitr verify --step N` exits 0 only on a byte-level sha256 match.**

<h2><img src="https://api.iconify.design/tabler:topology-star-3.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> Architecture</h2>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./assets/atlas-dark.svg">
  <source media="(prefers-color-scheme: light)" srcset="./assets/atlas-light.svg">
  <img src="./assets/atlas-light.svg" width="880" alt="Architecture: Agent Workspace → Watcher/RedoLog → Snapshotter/Store → Audit Ledger">
</picture>

Over the last 6–18 months the Coding Agent graduated from toy to long-chain, multi-step autonomous sessions — the blast radius of a single step grew from "one file" to "the whole working set polluted" as agents touched `/etc`, `~/.config`, and sqlite, state that git cannot reach and VM snapshots cannot hit at step granularity. The gap this exposes is the one the [ChromeDevTools/chrome-devtools-mcp](https://github.com/ChromeDevTools/chrome-devtools-mcp) crowd keeps circling: tooling that lets a Coding Agent act on the host, with nothing to roll the working set back per step. Agent orchestration platforms like [langgenius/dify](https://github.com/langgenius/dify) made the Agent a first-class platform, but nobody built the per-step transaction log and rewind primitive for the filesystem the agent edits. pitrrwnd maps the database redo-log point-in-time-recovery protocol onto a directory tree: one COW savepoint per agent step with a sha256 manifest, an fsnotify watcher writing a redo log between savepoints, rewind to any step, byte-level verify. The primitive is mature (DB PITR); it only became worth building once agent sessions grew long and autonomous enough.

## Table of contents

- [Why this exists](#why-this-exists)
- [Install & quickstart](#install--quickstart)
- [Usage](#usage)
- [Demo](#demo)
- [Roadmap](#roadmap)
- [Paid](#paid)
- [Share](#share)
- [License](#license)

## Why this exists

Today ops can only `git reset` (which can't reach untracked config/runtime state) or take whole-disk VM snapshots (coarse, not step-bound, no audit, space-heavy). 等保 2.0 requires "auditable restoration to the pre-intervention state", but the config/DB/runtime state an agent mutates is neither in git nor reachable at step granularity by whole-disk snapshots. pitrrwnd makes a per-step savepoint on the box, rewinds the working set byte-level to that step, and the redo log is the audit trail by construction — all without leaving the box.

## Install & quickstart

<h3><img src="https://api.iconify.design/tabler:rocket.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> Build from source (before first release)</h3>

```bash
git clone https://github.com/SuperMarioYL/pitrrwnd && cd pitrrwnd
make build
./scripts/demo.sh            # end-to-end: savepoint → agent breaks → rewind → verify exits 0
```

Once the first release is out, grab the prebuilt binary (static Go, zero runtime deps):

```bash
curl -L https://github.com/SuperMarioYL/pitrrwnd/releases/latest/download/pitr-linux-amd64.tar.gz | tar xz
sudo mv pitr /usr/local/bin/
```

<details><summary>Sample output (excerpt of scripts/demo.sh)</summary>

```
==> pitr savepoint --label "before risky refactor"
savepoint step=1 label="before risky refactor" working_set_sha256=0cf9147b678ce60533a25e23fc77f9eeb3810447a4b7e971907ad74e662dc704
==> pitr verify --step 1   (should FAIL)
verify step 1: FAIL — verify: app.conf sha256 mismatch (want 808a.., got 4c51..)
==> pitr rewind --step 1
rewound to step 1 ("before risky refactor"); working_set_sha256=0cf9147b678ce60533a25e23fc77f9eeb3810447a4b7e971907ad74e662dc704
==> pitr verify --step 1   (should pass, byte-level equal)
verify step 1: OK (working set sha256 matches manifest)
==> pitr audit-export -o bundle.tar.gz
audit bundle written: bundle.tar.gz
  audit_sha256=aec87c191289500ea95fba98ad23176fd28d086f4d4893af25af08c93604a516
```
</details>

## Usage

<h2><img src="https://api.iconify.design/tabler:terminal-2.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> Usage</h2>

All state lives under `.pitr/` in the workspace root — single process, foreground, no daemon. `pitr watch` runs in the foreground while the agent works; `savepoint` / `rewind` / `verify` / `log` / `audit-export` are stateless subcommands over `.pitr/`.

```bash
# 1. Initialize transactional state in the agent workspace
cd ~/agent-work && pitr init

# 2. Take a savepoint before the agent runs (COW snapshot + sha256 manifest)
pitr savepoint --label "before risky refactor"

# 3. Let the agent run (optionally `pitr watch` in another terminal to record per-write redo)
#    ...agent corrupts app.conf, deletes db/store.db, leaves junk.txt...

# 4. Rewind to step 1 and verify byte-level equality
pitr rewind --step 1
pitr verify --step 1            # exits 0 only on a full sha256 match

# 5. Inspect the timeline + audit trail, export the reviewer bundle
pitr log
pitr audit-export -o bundle.tar.gz
```

### Commands & flags

| Command | Key flags | What it does |
|---|---|---|
| `pitr init` | `--root/-C` | Create the `.pitr/` state dir (bbolt index + snapshots + manifests + redo.wal + audit.jsonl) |
| `pitr watch` | `--root/-C` | Foreground fsnotify watcher; appends a redo entry per file write (Ctrl-C to stop) |
| `pitr savepoint` | `--label/-l` | Write a reflink/COW snapshot + per-file sha256 manifest; truncate the redo log to the boundary |
| `pitr rewind` | `--step/-s N` | Restore the working set from the snapshot, drop agent-added files, append a rewind audit entry |
| `pitr verify` | `--step/-s N` | Hash every file against the manifest; exit 0 only on full match (the killer check) |
| `pitr log` | `--root/-C` | Print the savepoint timeline + the append-only audit trail |
| `pitr audit-export` | `--out/-o` | Emit a tar.gz reviewer bundle (audit.jsonl + savepoints + manifest sha256 + version) |

`--root/-C` sets the workspace root on any subcommand (defaults to the current directory).

## Demo

<h2><img src="https://api.iconify.design/tabler:photo.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> Demo</h2>

Ten minutes end-to-end: `pitr init` → take a savepoint → simulate an agent corrupting config / deleting a db / dropping junk files → `pitr rewind --step 1` → `pitr verify --step 1` exits 0 byte-equal.

![demo](assets/demo.gif)

The full reproducible script is [`scripts/demo.sh`](./scripts/demo.sh); the vhs tape is [`docs/demo.tape`](./docs/demo.tape) (`make demo` re-renders the gif).

### How it compares

| Capability | pitrrwnd | `git reset` / worktree | VM / container snapshot |
|---|:---:|:---:|:---:|
| Covers non-git-tracked files (`/etc`, DB, runtime state) | ✓ | — | ✓ |
| Per agent-step granularity | ✓ | — (human-commit) | — (whole-disk) |
| Byte-level sha256 verify exit code | ✓ | — | — |
| Append-only audit trail (sha256 + ISO ts + host + user) | ✓ | — | partial |
| Fully local, airgap-safe, static binary, zero network deps | ✓ | ✓ | partial |
| Distributed collaboration / branch semantics | — | ✓ (git wins) | — |

Honestly: git wins by a mile on distributed collaboration and branch semantics — pitrrwnd does not replace git, it fills the per-step byte-level rewind + audit gap git structurally cannot reach.

## Roadmap

<h2><img src="https://api.iconify.design/tabler:map-2.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> Roadmap</h2>

- [x] **m1 snapshot & savepoint** — `pitr init` builds `.pitr/`; `pitr savepoint` writes a reflink/COW snapshot + sha256 manifest; `pitr verify --step N` exits 0 (ext4 + XFS).
- [x] **m2 rewind & restore** — `pitr rewind --step N` restores the working set from the snapshot, appends an audit entry; `verify --step N` byte-level-equal.
- [x] **m3 log, audit, demo** — `pitr log` timeline + audit trail; `pitr audit-export` reviewer bundle; `scripts/demo.sh` reproducible.
- [ ] Sub-savepoint redo-log replay (`pitr rewind --to TIMESTAMP`, v0.2)
- [ ] Claude Code / Cursor wrapper (`scripts/claude-code-hook.sh`, v0.2)
- [ ] LoongArch prebuilt binaries
- [ ] Snapshot-store dedup / compression (beyond hardlinks)
- [ ] Encrypted-at-rest snapshot store

## Paid

<h2><img src="https://api.iconify.design/tabler:cash.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> Paid</h2>

The core PITR engine and the local `.pitr/audit.jsonl` + `audit-export` reviewer bundle are open source forever (MIT) — the single-box static binary is fully usable and core rewind is never gated.

For 信创/等保 compliance ops and on-prem-LLM multi-box agent fleets, the **team plan** runs inside the airgap (no cloud):

- **Multi-box central audit dashboard** aggregating each box's `.pitr/audit.jsonl` into one 等保-reviewable view
- **LoongArch prebuilt binaries**
- **等保 reviewer-bundle templates** (signable) + SLA + on-prem license key
- **Pricing**: ¥6,800 / box / year, 5-box minimum (¥34k/yr floor); first 3 信创 customers Q4 pilot at ¥3,000 / box year one
- **Billing**: bank-transfer invoice + license-key gating (airgap forbids Stripe / cloud metering)
- **Smallest "yes" path**: a 30-minute remote session on their staging 信创 box — `pitr init` → agent breaks a config → `pitr rewind --step 1` + `pitr audit-export` → 等保 reviewer signs the bundle on the spot → PO within 14 days

For pilots or commercial enquiries, open an issue tagged `commercial` or email.

## Share

```
pitrrwnd — filesystem PITR for Coding Agent worksets. Database redo-log mapped onto a directory tree: per-step byte-level rewind to any point before the agent ran, verify exits 0 or it didn't restore, all local with an audit trail. https://github.com/SuperMarioYL/pitrrwnd
```

## License

<h2><img src="https://api.iconify.design/tabler:license.svg?color=%230071E3&width=24" height="22" align="absmiddle" alt=""> License</h2>

MIT — see [LICENSE](./LICENSE). Issues and PRs welcome; for commercial / compliance needs see [Paid](#paid) above.

<p align="center"><sub><a href="./LICENSE">MIT</a> © 2026 SuperMarioYL</sub></p>
