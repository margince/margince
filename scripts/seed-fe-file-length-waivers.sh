#!/usr/bin/env bash
# Re-freeze the frontend length waivers at the tree's current sizes.
#
# The baseline is "the tree as it stands", so it is taken when the gate lands —
# and until then every merge to main moves it. This is that operation as a
# command rather than a one-liner somebody retypes. The population comes from
# the gate itself, run against an empty ledger, so neither the roots it walks
# nor the wider ceiling it gives a test is spelled twice. Restating them here
# is what let the gate learn frontend/scripts while this script still wrote
# only the screens.
#
# It is NOT a way past a failing gate. Once the gate is in force, a file that
# grew is a file to shrink, and running this would freeze the growth instead —
# which is the one thing the ratchet exists to refuse. Use it to establish the
# baseline, and to rebase it before the gate first lands.
set -euo pipefail
cd "$(dirname "$0")/.."

WAIVERS="${FE_FILE_LENGTH_WAIVERS:-scripts/fe-file-length-waivers.txt}"
GATE="$(dirname "$0")/check-fe-file-length.sh"

# The gate's own report over an empty ledger: one FAIL line per file above its
# cap, carrying the count this ledger freezes.
set +e
report="$(FE_FILE_LENGTH_WAIVERS=/dev/null bash "$GATE" 2>&1)"
status=$?
set -e
# A clean tree exits 0 and the ledger comes out empty. Any other non-zero
# without the gate's own summary line is the gate broken rather than a tree
# over the cap, and freezing a ledger from that report would write nothing and
# call it a baseline.
if [[ "$status" -ne 0 ]] && ! grep -q "^FAIL: fe-file-length" <<<"$report"; then
  echo "$report" >&2
  echo "seed-fe-file-length-waivers: the gate did not run, so there is no baseline to freeze" >&2
  exit 1
fi

{
  cat <<'HEAD'
# Pre-existing frontend files over the cap, frozen at their line count when the
# gate landed (scripts/check-fe-file-length.sh). Shrinking is allowed; growing
# is not. When a file drops to the cap or below, DELETE its line here.
#
# The cap is 500 for product code and 1000 for a test, story, testkit or
# fixture file. This list is the frontend's size debt as it stood, and the only
# direction it may move is shorter.
#
# Regenerate with scripts/seed-fe-file-length-waivers.sh — to ESTABLISH this
# baseline, never to absorb a file that grew.
#
# One exception: a PR that only moves modules may raise a line by the import
# lines its rewritten paths added, and by nothing else. Its PR body lists each
# raise.
#
# The population is the two trees the gate walks: frontend/src and
# frontend/scripts.
# Format: <frozen-line-count> <path>
HEAD
  sed -n 's/^FAIL: \(.*\) is \([0-9][0-9]*\) lines (> .*$/\2 \1/p' <<<"$report" \
  | sort -k2
} > "$WAIVERS"

echo "seeded $WAIVERS with $(grep -cvE '^\s*#|^\s*$' "$WAIVERS") entries"
