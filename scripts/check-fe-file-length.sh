#!/usr/bin/env bash
# Frontend file-length gate with a ratchet, the counterpart to
# scripts/check-go-file-length.sh. The craftsmanship ceilings CLAUDE.md states
# were enforced on the Go half of the product and on nothing else, so a screen
# could grow without limit while a package of the same size failed a push.
#
# The ratchet: scripts/fe-file-length-waivers.txt records each pre-existing
# offender with its frozen line count. A waived file may shrink but never grow
# past its recorded count; once it drops to the cap or below, its entry must be
# REMOVED so the file is back under the hard cap for good.
#
# THE CAPS ARE THE GO ONES — 500 for product code, 1000 for a test or a story.
# JSX is more verbose per unit of logic than Go, which is an argument for a
# higher number and not a good one: the ceiling asks how much a reader must hold
# at once, and that is a property of the reader. A 900-line component is 900
# lines to read whichever language wrote it. The waiver list is what admits the
# tree as it stands, and its length is the measure of the debt rather than an
# argument for a laxer cap.
#
# EXEMPT, and why each is not a loophole:
#   *.d.ts and src/api/public-events.ts — generated from the contract, the same
#     exemption *_gen.go carries. Splitting them would be splitting the
#     generator's output.
#   src/i18n/{en,de,vi}.ts — message catalogs. They are DATA: one entry per key,
#     read by lookup and never top to bottom, and cutting them by size would
#     scatter a translation across files a translator has to find.
set -euo pipefail
cd "$(dirname "$0")/.."

CAP="${FE_FILE_LINE_CAP:-500}"
TEST_CAP="${FE_TEST_FILE_LINE_CAP:-1000}"
WAIVERS="scripts/fe-file-length-waivers.txt"

find frontend/src \( -name "*.ts" -o -name "*.tsx" \) \
    ! -name "*.d.ts" \
    ! -path "frontend/src/api/public-events.ts" \
    ! -path "frontend/src/i18n/en.ts" \
    ! -path "frontend/src/i18n/de.ts" \
    ! -path "frontend/src/i18n/vi.ts" \
    -exec wc -l {} + \
| awk -v cap="$CAP" -v testcap="$TEST_CAP" -v waivers="$WAIVERS" '
BEGIN {
  while ((getline line < waivers) > 0) {
    if (line ~ /^[[:space:]]*#/ || line ~ /^[[:space:]]*$/) continue
    split(line, a, " ")
    waived[a[2]] = a[1] + 0
  }
  close(waivers)
}
# A test, a story or a fixture reads as a spec and carries its own data, so it
# takes the wider ceiling — the same split the Go gate makes.
#
# The tree spells fixtures FOUR ways: `x.fixtures.ts`, a bare `fixtures.ts`, a
# singular `fixture.ts` and a hyphenated `test-fixtures.ts`. A predicate that
# knew only the dotted form would hand the other three the product cap, which
# is a rule nobody wrote applied to files nobody meant.
function istest(f) {
  return f ~ /\.test\.tsx?$/ ||
         f ~ /\.stories\.tsx?$/ ||
         f ~ /\.testkit\.tsx?$/ ||
         f ~ /(^|[\/.-])fixtures?\.tsx?$/
}
$2 == "total" { next }
{
  lines = $1 + 0
  file = $2
  limit = istest(file) ? testcap : cap
  if (file in waived) {
    seen[file] = 1
    if (lines > waived[file]) {
      printf "FAIL: %s grew to %d lines (waiver froze it at %d) — shrink it, never grow it\n", file, lines, waived[file]
      fail = 1
    } else if (lines <= limit) {
      printf "FAIL: %s is down to %d lines (<= %d) — remove its stale entry from %s so the cap re-arms\n", file, lines, limit, waivers
      fail = 1
    }
  } else if (lines > limit) {
    printf "FAIL: %s is %d lines (> %d) — split it into one concept per file\n", file, lines, limit
    fail = 1
  }
}
END {
  for (f in waived) if (!(f in seen)) {
    printf "FAIL: waiver entry for missing file %s — remove it from %s\n", f, waivers
    fail = 1
  }
  if (fail) {
    printf "FAIL: fe-file-length — the %d-LOC cap (%d for a test or story), ratcheted via %s\n", cap, testcap, waivers
    exit 1
  }
  n = 0; for (f in waived) n++
  printf "OK: fe-file-length — no frontend source file exceeds %d LOC (%d for a test or story) (waivers: %d)\n", cap, testcap, n
}
'
