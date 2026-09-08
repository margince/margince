#!/usr/bin/env bash
# Re-freeze the frontend length waivers at the tree's current sizes.
#
# The baseline is "the tree as it stands", so it is taken when the gate lands —
# and until then every merge to main moves it. This is that operation as a
# command rather than a one-liner somebody retypes: it writes exactly what
# check-fe-file-length.sh reads, including the exemptions, so the two cannot
# disagree about which files are in the population.
#
# It is NOT a way past a failing gate. Once the gate is in force, a file that
# grew is a file to shrink, and running this would freeze the growth instead —
# which is the one thing the ratchet exists to refuse. Use it to establish the
# baseline, and to rebase it before the gate first lands.
set -euo pipefail
cd "$(dirname "$0")/.."

CAP="${FE_FILE_LINE_CAP:-500}"
TEST_CAP="${FE_TEST_FILE_LINE_CAP:-1000}"
WAIVERS="scripts/fe-file-length-waivers.txt"

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
# Format: <frozen-line-count> <path>
HEAD
  find frontend/src \( -name "*.ts" -o -name "*.tsx" \) \
      ! -name "*.d.ts" \
      ! -path "frontend/src/api/public-events.ts" \
      ! -path "frontend/src/i18n/en.ts" \
      ! -path "frontend/src/i18n/de.ts" \
      ! -path "frontend/src/i18n/vi.ts" \
      -exec wc -l {} + \
  | awk -v cap="$CAP" -v testcap="$TEST_CAP" '
      $2 == "total" { next }
      {
        lines = $1 + 0
        f = $2
        limit = (f ~ /\.test\.tsx?$/ || f ~ /\.stories\.tsx?$/ || f ~ /\.testkit\.tsx?$/ || f ~ /(^|[\/.-])fixtures?\.tsx?$/) ? testcap : cap
        if (lines > limit) printf "%d %s\n", lines, f
      }' \
  | sort -k2
} > "$WAIVERS"

echo "seeded $WAIVERS with $(grep -cvE '^\s*#|^\s*$' "$WAIVERS") entries"
