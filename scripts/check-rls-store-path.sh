#!/usr/bin/env bash
# Store-path gate for database.WithTx.
#
# The rule: a module's statement addresses the TRANSACTION (`tx.Exec`/`tx.Query`),
# never `<recv>.pool.{Exec,Query,QueryRow}`. What that buys is the write shape the
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

dir="backend/internal/modules"

# A scan that reads far fewer files than the tree holds would pass with no
# subject, so the floor is fixed here and raised when the tree grows.
min_files=1000

# One awk pass over every non-test module .go file; prev resets per file. It
# matches both spellings the tree uses: the field `.pool.Exec(` and the accessor
# `.Pool().Exec(`. It also counts every match, waived or not.
files="$(find "$dir" -name '*.go' ! -name '*_test.go' | sort)"
file_count="$(echo "$files" | wc -l | tr -d ' ')"
if (( file_count < min_files )); then
  echo "FAIL: rls-store-path scanned $file_count module files, fewer than the floor of $min_files; the scan lost the module tree"
  exit 1
fi

scan="$(echo "$files" | xargs awk '
  FNR == 1 { prev = "" }
  $0 ~ /(\.pool|\.Pool\(\))\.(ExecContext|QueryContext|QueryRowContext|Exec|Query|QueryRow|Begin|BeginTx|Acquire|SendBatch|CopyFrom)\(/ {
    matched++
    if (prev !~ /\/\/[[:space:]]*pool-exempt:/) {
      line = $0; sub(/^[[:space:]]+/, "", line)
      printf "%s:%d: %s\n", FILENAME, FNR, line
    }
  }
  { prev = $0 }
  END { printf "MATCHED %d\n", matched }
')"
matched="$(echo "$scan" | sed -n 's/^MATCHED //p')"
violations="$(echo "$scan" | grep -v '^MATCHED ' || true)"
if (( matched < 1 )); then
  echo "FAIL: rls-store-path matched no pool statement at all; the pattern no longer sees the spellings the tree uses"
  exit 1
fi

if [[ -n "$violations" ]]; then
  echo "FAIL: module statements addressing the pool directly, outside the transaction seam:"
  echo "$violations"
  echo
  echo "Route each through database.WithTx and address the tx, not the pool, so the"
  echo "domain row commits with its audit and outbox rows. For a statement that must run"
  echo "outside any transaction, add a '// pool-exempt: <reason>' line above it."
  exit 1
fi

echo "OK: rls-store-path: no module statement addresses the pool directly"
