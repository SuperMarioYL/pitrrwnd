#!/usr/bin/env bash
#
# pitrrwnd reproducible end-to-end demo.
#
# Builds the pitr binary (if missing), spins up a throwaway agent workspace,
# marks a savepoint, simulates a destructive agent edit, rewinds, and verifies
# byte-level equality — the killer demo. Run: scripts/demo.sh
#
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
BIN="$ROOT/bin/pitr"

if [ ! -x "$BIN" ]; then
	echo "building pitr at $BIN ..."
	(cd "$ROOT" && go build -trimpath -ldflags "-X main.version=$(cat "$ROOT/VERSION")" -o "$BIN" ./cmd/pitr)
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "==> workspace: $WORK"
cd "$WORK"

echo "==> pitr init"
"$BIN" init

echo "==> seed working set"
printf 'port: 8080\n' > app.conf
printf 'v1\n' > version.txt
mkdir -p db && printf 'rows=42\n' > db/store.db

echo "==> pitr savepoint --label \"before risky refactor\""
"$BIN" savepoint --label "before risky refactor"

echo "==> simulate a 3am agent command: edits config, drops junk, deletes the db"
printf 'port: 9999\nBROKEN\n' > app.conf
printf 'the agent made this\n' > agent-junk.txt
rm db/store.db

echo "==> pitr verify --step 1   (should FAIL)"
"$BIN" verify --step 1 || true

echo "==> pitr rewind --step 1   (restore to before the agent)"
"$BIN" rewind --step 1

echo "==> pitr verify --step 1   (should pass, byte-level equal)"
"$BIN" verify --step 1

echo "==> pitr log"
"$BIN" log

echo "==> pitr audit-export -o bundle.tar.gz"
"$BIN" audit-export -o bundle.tar.gz
echo "bundle contents:"
tar tzf bundle.tar.gz

echo
echo "==> demo complete: working set byte-level-equal to step 1, audit trail recorded locally."
