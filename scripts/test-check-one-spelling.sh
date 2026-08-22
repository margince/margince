#!/usr/bin/env bash
# Prove check-one-spelling.sh fires on each defect it names, stays silent on
# the lookalikes that are not defects, and honours its own waiver.
#
# A gate nobody has watched fail is a gate nobody knows the shape of. Each case
# below plants ONE instance in a throwaway file inside the scanned tree, runs
# the real script, and asserts on its verdict — so a regex edit that silently
# stops matching, or starts matching a bystander, fails here rather than in
# somebody's push six weeks later.
set -uo pipefail
cd "$(dirname "$0")/.."

GATE=./scripts/check-one-spelling.sh

# The probes live OUTSIDE the repository, and the gate is pointed at them with
# ONE_SPELLING_SCAN. Planting a deliberate defect in the real tree would make
# `make -j` a race: a concurrent one-spelling would report the probe, and
# gofmt, the license gate and craft would each see an unlicensed stray file.
PROBE_DIR="$(mktemp -d)"
PLANT="$PROBE_DIR/zz_one_spelling_probe.go"
trap 'rm -rf "$PROBE_DIR"' EXIT
fails=0

# plant <body> — write a compiling-shaped probe file carrying <body>.
plant() { printf '// SPDX-License-Identifier: BUSL-1.1\npackage storekit\n\n%s\n' "$1" > "$PLANT"; }

# expect <fires|silent> <name> <body> [must-report] [must-not-report]
#
# The two optional arguments are what make a MIXED probe worth planting. A case
# that plants a waived defect beside an unwaived one and only checks the exit
# status passes just as well from a scanner that reports BOTH — the waiver could
# be doing nothing and the run would still be red for the right overall reason.
# So the case names the finding it must see and the one it must not.
expect() {
  local want="$1" name="$2" body="$3" must="${4:-}" mustnot="${5:-}" out rc
  plant "$body"
  out="$(ONE_SPELLING_SCAN="$PROBE_DIR" $GATE 2>&1)"; rc=$?
  rm -f "$PLANT"
  if [[ "$want" == fires && $rc -eq 0 ]]; then
    echo "FAIL: $name — the gate passed over it"; echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  if [[ "$want" == silent && $rc -ne 0 ]]; then
    echo "FAIL: $name — the gate refused it"; echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  if [[ -n "$must" ]] && ! grep -qF -- "$must" <<< "$out"; then
    echo "FAIL: $name — it never reported $must, so the red came from something else"
    echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  if [[ -n "$mustnot" ]] && grep -qF -- "$mustnot" <<< "$out"; then
    echo "FAIL: $name — it reported $mustnot, which is waived"
    echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  echo "ok: $name"
}

echo "== the tree as it stands =="
if ! $GATE >/dev/null 2>&1; then
  echo "FAIL: the gate does not pass on an unmodified tree — every case below is unreadable"
  $GATE 2>&1 | sed 's/^/    /'
  exit 1
fi
echo "ok: clean tree passes"

echo
echo "== each arm fires on its own defect =="
expect fires "SQLSTATE literal"        'func probe(c string) bool { return c == "23505" }' '23505'
expect fires "CHECK-to-422 re-spelling" 'const probeCode = "constraint_violated"' 'constraint_violated'
expect fires "private ISO-4217 regexp"  'const probeShape = `^[A-Z]{3}$`' 'A-Z'

echo
echo "== a SQLSTATE the gate was never told about, read from sqlstate.go =="
# 22001 is NOT in sqlstate.go, so it must NOT fire — the list is derived, and a
# hand-typed list would have had to guess which codes matter.
expect silent "an unrelated SQLSTATE-shaped literal" 'const probeLen = "22001"'

echo
echo "== the waiver, and what must stay silent without one =="
expect silent "a waived line" \
  'func probe(c string) bool { return c == "23505" } // one-spelling-exempt: probing the gate'

# The exception is the attack surface. A waiver must silence the LINE it is on
# and nothing else — a waiver that quiets the file would let the next defect in
# free, under a reason written about something else entirely.
expect fires "a waiver silences its own line only" \
  'func waived(c string) bool { return c == "23505" } // one-spelling-exempt: probing the gate
func notWaived(c string) bool { return c == "23503" }' \
  '23503' '23505'
expect silent "the same token in a line comment" \
  '// A dedupe hit is "23505", named in sqlstate.go.'
expect silent "the same token in a block comment" \
  '/*
A dedupe hit is "23505" and the wire code was "constraint_violated".
*/'
expect silent "the same token in an inline block comment" \
  'const probeN = 1 /* not "23505" */'

echo
echo "== a comment is not a string, and a string is not a comment =="
# The gate's waiver used to be `the marker appears somewhere on this line`, so a
# marker inside a STRING silenced the defect beside it — a waiver nobody wrote
# as one. Without this case the whole reading can be reverted and the suite
# stays green, which is how the bypass survived its own review.
expect fires "a waiver forged inside a string literal" \
  'func probe(c string) (bool, string) { return c == "23505", "one-spelling-exempt: fake" }'

# No forgery needed for this one. The strip pass used to cut the line at the
# first ` //`, so a string carrying one hid every defect after it — and 167
# lines in this tree already carry a `//` inside a string, mostly //nolint:
# directives quoted in prose and URL paths.
expect fires "a  //  inside a string does not truncate the line" \
  'func probe(c string) bool { path := "/oauth // token"; _ = path; return c == "23505" }'

# A Go raw string spans lines, so a scanner reading one line at a time takes
# the CLOSING backtick for an opening quote and swallows the trailing comment
# as string content. Both directions were live: a truthful comment became a
# finding, and a waiver on such a line stopped being read at all.
expect silent "a comment on the line closing a raw string" \
  'const query = `SELECT 1
FROM person` // the store maps "23505" via storekit, never here'
expect silent "a waiver on the line closing a raw string" \
  'const query = `SELECT 1
FROM person` + probe("23505") // one-spelling-exempt: seeding the dedupe fixture'

# ...and the inverse, which is the direction that costs a gate everything: a
# raw string must CLOSE. If the scanner stays inside one, every line after it
# is read as string content and the rest of the file is silently exempt.
expect fires "a defect after a multi-line raw string" \
  'const query = `SELECT 1
FROM person`
func probe(c string) bool { return c == "23505" }'
expect fires "a defect after a raw string ending in a backslash" \
  'const winPath = `C:\\tmp\\`
func probe(c string) bool { return c == "23505" }'

# SQL text inside a raw string compares against the code in SQL quotes, and
# storekit is not reachable from a query string — so this stays silent, and did
# before the state was carried across lines too. Pinned because carrying it is
# exactly the change that could start reading query text as Go.
expect silent "a SQLSTATE in SQL text inside a raw string" \
  "const query = \`SELECT 1
WHERE sqlstate = '23505'
FROM person\`"

echo
if [[ $fails -eq 1 ]]; then
  echo "FAIL: check-one-spelling.sh does not behave as its header claims"
  exit 1
fi
echo "OK: check-one-spelling.sh fires on each defect, stays silent on each lookalike"
