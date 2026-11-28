#!/usr/bin/env bash
# Store-path gate for database.WithTx.
#
# The rule: a statement in a module or in compose addresses the TRANSACTION
# (`tx.Exec`/`tx.Query`), never a pool. Compose is held as modules are, because it
# writes domain tables through the same seams. What that buys is the write shape the
# whole product rests on: a domain row, its audit_log row and its event_outbox row
# commit together or not at all. That only holds if the domain statement is inside
# the same transaction the audit and outbox writes are. A statement on the bare
# pool is outside it: it commits on its own, before or after the work it belongs
# to, and no later reader can tell the difference from a trace.
#
# Every domain read and write reaches the database through database.WithTx (or a
# handle's Tx), so "what touches domain data, and under what boundary" has a single
# answer rather than a grep.
#
# It runs in the deterministic lane: no database, and it fails fast when a store
# addresses the pool directly. The one escape hatch is a `// pool-exempt: <reason>`
# comment on the line immediately above a statement that must run on the bare
# pool, for example deployment metadata that no audit row describes. Use it
# sparingly.
set -euo pipefail
cd "$(dirname "$0")/.."

# Platform is left out: it is the transaction seam itself, and what it may write
# is bounded by backend/gates/tableownership_test.go, which walks its writers.
# A scan that reads far fewer files than a tree holds would pass with no subject,
# so each tree has a floor, raised when the tree grows.
trees="backend/internal/modules:3000 backend/internal/compose:2000"

# A file built only with the integration tag is a test fixture, never product.
files=""
for tree in $trees; do
  dir="${tree%%:*}" floor="${tree##*:}"
  found="$(find "$dir" -name '*.go' ! -name '*_test.go' -exec grep -L '^//go:build integration$' {} + | sort)"
  count="$(echo "$found" | grep -c . || true)"
  if (( count < floor )); then
    echo "FAIL: rls-store-path scanned $count files in $dir, fewer than the floor of $floor; the scan lost the tree"
    exit 1
  fi
  files="$files$found"$'\n'
done

# One awk pass; prev resets per file. Any receiver whose name ends in pool or
# Pool counts, so `pool.`, `.pool.`, `.schemaPool.` and `.Pool().` all match. It
# also counts every match, waived or not.
scan="$(echo "$files" | grep . | xargs awk '
  FNR == 1 { prev = "" }
  $0 ~ /[[:alnum:]_]*[Pp]ool(\(\))?\.(ExecContext|QueryContext|QueryRowContext|Exec|Query|QueryRow|Begin|BeginTx|Acquire|SendBatch|CopyFrom)\(/ {
    matched++
    if (prev !~ /\/\/[[:space:]]*pool-exempt:/) {
      line = $0; sub(/^[[:space:]]+/, "", line)
      printf "%s:%d: %s\n", FILENAME, FNR, line
    }
  }
  { prev = $0 }
  END { printf "MATCHED %d\n", matched }
')"
# xargs may split the list over several awk runs, each printing its own count.
matched="$(echo "$scan" | sed -n 's/^MATCHED //p' | awk '{ total += $1 } END { print total + 0 }')"
violations="$(echo "$scan" | grep -v '^MATCHED ' || true)"
if (( matched < 1 )); then
  echo "FAIL: rls-store-path matched no pool statement at all; the pattern no longer sees the spellings the tree uses"
  exit 1
fi

if [[ -n "$violations" ]]; then
  echo "FAIL: module or compose statements addressing a pool directly, outside the transaction seam:"
  echo "$violations"
  echo
  echo "Route each through database.WithTx and address the tx, not the pool, so the"
  echo "domain row commits with its audit and outbox rows. For a statement that must run"
  echo "outside any transaction, add a '// pool-exempt: <reason>' line above it."
  exit 1
fi

echo "OK: rls-store-path: no module or compose statement addresses a pool directly"
