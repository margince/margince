#!/usr/bin/env bash
# Store-path gate for this repo's WithWorkspaceTx seam.
#
# The rule is seam discipline: a module's statement addresses the TRANSACTION
# (`tx.Exec`/`tx.Query`), never `<recv>.pool.{Exec,Query,QueryRow}`. What that
# buys is the write shape the whole product rests on — a domain row, its
# audit_log row and its event_outbox row commit together or not at all — and
# that only holds if the domain statement is inside the same transaction the
# audit and outbox writes are. A statement on the bare pool is outside it: it
# commits on its own, before or after the work it belongs to, and no later
# reader can tell the difference from a trace.
#
# The second thing it buys is that there is ONE place to audit. Every domain
# read and write reaches the database through database.WithWorkspaceTx, so
# "what touches tenant data, and under what boundary" is a question with a
# single answer rather than a grep.
#
# It is a deterministic-lane gate: no database, fails fast when a store
# addresses the pool directly. The sole sanctioned escape hatch
# — a genuinely system-wide query (e.g. the worker loops that enumerate before
# entering a per-unit-of-work tx) — is a `// rls-exempt: <reason>` comment on
# the line immediately above. Use it sparingly. The hatch's name is older than
# the rule's current rationale and is kept because renaming it would rewrite
# every existing use for no gain in enforcement.
set -euo pipefail
cd "$(dirname "$0")/.."

dir="backend/internal/modules"

# One awk pass over every non-test module .go file; prev resets per file.
files="$(find "$dir" -name '*.go' ! -name '*_test.go' | sort)"
violations="$(echo "$files" | xargs awk '
  FNR == 1 { prev = "" }
  $0 ~ /\.pool\.(ExecContext|QueryContext|QueryRowContext|Exec|Query|QueryRow)\(/ {
    if (prev !~ /\/\/[[:space:]]*rls-exempt:/) {
      line = $0; sub(/^[[:space:]]+/, "", line)
      printf "%s:%d: %s\n", FILENAME, FNR, line
    }
  }
  { prev = $0 }
')"

if [[ -n "$violations" ]]; then
  echo "FAIL — module statements addressing the pool directly, outside the transaction seam:"
  echo "$violations"
  echo
  echo "Route each through database.WithWorkspaceTx and address the tx, not the pool, so the"
  echo "domain row commits with its audit and outbox rows. For a genuinely system-wide query,"
  echo "add a '// rls-exempt: <reason>' line above it."
  exit 1
fi

echo "OK: rls-store-path — no module statement addresses the superuser pool directly"
