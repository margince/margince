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
# Resolved BEFORE the cd. `$0` is whatever the caller typed, so after changing
# directory it can name a path that does not exist — and the census below then
# reads zero cases and reports a fully passing run as a failure. Launching the
# suite from `scripts/` did exactly that.
SELF="$(cd -P -- "$(dirname -- "$0")" && pwd)/$(basename -- "$0")"
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
ran=0

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
  # A non-zero exit is not the same as a DETECTION. The gate also refuses when it
  # could not finish reading a file, and a `fires` case that reads only the
  # status would score that as the defect being caught — so every detection case
  # in this suite could be satisfied by a scanner that has stopped working.
  if [[ "$want" == fires ]] && grep -q "still inside a string or a block comment" <<< "$out"; then
    echo "FAIL: $name — the gate refused because it could not READ the probe, not because of what it plants"
    echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  if [[ "$want" == fires && $rc -eq 0 ]]; then
    echo "FAIL: $name — the gate passed over it"; echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  # `unclosed` is its own expectation and not a flavour of `fires`, because the
  # two refusals mean opposite things about the run: `fires` says the gate READ
  # the code and found the defect, `unclosed` says it could not read the code at
  # all and refused rather than pretend. Scoring one as the other would let a
  # scanner that has stopped working satisfy every detection case in the suite.
  if [[ "$want" == unclosed ]] && ! grep -q "still inside a string or a block comment" <<< "$out"; then
    echo "FAIL: $name — the gate did not refuse the unreadable file (exit $rc)"
    echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  # And it must NAME the file. The whole point of refusing rather than passing
  # is that the reader can go and look, so the diagnostic contract is the path,
  # not the sentence around it — a refusal that says only "some file somewhere"
  # tells them nothing they can act on.
  if [[ "$want" == unclosed ]] && ! grep -qF -- "$PLANT" <<< "$out"; then
    echo "FAIL: $name — the gate refused but did not say WHICH file it could not read"
    echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  if [[ "$want" == unclosed && $rc -eq 0 ]]; then
    echo "FAIL: $name — the gate reported OK over a file it never finished reading"
    echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  if [[ "$want" == silent && $rc -ne 0 ]]; then
    echo "FAIL: $name — the gate refused it"; echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  # `must` is REQUIRED for a detection case, not optional. The gate has three
  # arms and a fourth refusal for a file it could not read, so "exited non-zero"
  # is four different sentences — and a case that reads only the status passes
  # on any of them. Every `fires` case names the token it expects to see
  # reported, so the arm that fired is the arm being tested.
  if [[ "$want" == fires && -z "$must" ]]; then
    echo "FAIL: $name — a detection case must name the token it expects reported, or it passes on any refusal"
    fails=1; return
  fi
  if [[ -n "$must" ]] && ! grep -qF -- "$must" <<< "$out"; then
    echo "FAIL: $name — it never reported $must, so the red came from something else"
    echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  if [[ -n "$mustnot" ]] && grep -qF -- "$mustnot" <<< "$out"; then
    echo "FAIL: $name — it reported $mustnot, which is waived"
    echo "$out" | sed 's/^/    /'; fails=1; return
  fi
  ran=$((ran + 1))
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
  'func probe(c string) (bool, string) { return c == "23505", "one-spelling-exempt: fake" }' \
  '23505'

# No forgery needed for this one. The strip pass used to cut the line at the
# first ` //`, so a string carrying one hid every defect after it — and 167
# lines in this tree already carry a `//` inside a string, mostly //nolint:
# directives quoted in prose and URL paths.
expect fires "a  //  inside a string does not truncate the line" \
  'func probe(c string) bool { path := "/oauth // token"; _ = path; return c == "23505" }' \
  '23505'

# A Go raw string spans lines, so a scanner reading one line at a time takes
# the CLOSING backtick for an opening quote and swallows the trailing comment
# as string content. Both directions were live: a truthful comment became a
# finding, and a waiver on such a line stopped being read at all.
expect silent "a comment on the line closing a raw string" \
  'const query = `SELECT 1
FROM person` // the store maps "23505" via storekit, never here'
# Silence on that line is only half the claim. one-spelling KEEPS string
# contents, so a scanner that still misreads the closing backtick emits the
# `23505` from a later line and satisfies a bare `fires` — the state has to be
# shown closed, by planting a real defect after the string and requiring the
# gate to name THAT one.
expect fires "the raw-string state closes, so a later defect is still seen" \
  'const query = `SELECT 1
FROM person` // the store maps "23503" via storekit, never here

func probe(c string) bool { return c == "23505" }' \
  '23505' '23503'
expect silent "a waiver on the line closing a raw string" \
  'const query = `SELECT 1
FROM person` + probe("23505") // one-spelling-exempt: seeding the dedupe fixture'

# ...and the inverse, which is the direction that costs a gate everything: a
# raw string must CLOSE. If the scanner stays inside one, every line after it
# is read as string content and the rest of the file is silently exempt.
expect fires "a defect after a multi-line raw string" \
  'const query = `SELECT 1
FROM person`
func probe(c string) bool { return c == "23505" }' \
  '23505'
expect fires "a defect after a raw string ending in a backslash" \
  'const winPath = `C:\\tmp\\`
func probe(c string) bool { return c == "23505" }' \
  '23505'
# SQL text inside a raw string compares against the code in SQL quotes, and
# storekit is not reachable from a query string — so this stays silent, and did
# before the state was carried across lines too. Pinned because carrying it is
# exactly the change that could start reading query text as Go.
expect silent "a SQLSTATE in SQL text inside a raw string" \
  "const query = \`SELECT 1
WHERE sqlstate = '23505'
FROM person\`"

# A backslash is literal inside a Go raw string and an escape inside a
# TypeScript template, so one rule cannot serve both. Reading Go's as an escape
# ate the closing backtick: the NEXT backtick closed the string instead, quote
# parity inverted, and a `//` that was string content became a comment opener
# that discarded the defect after it. `\` is the Windows separator idiom and
# there are seven of them in this tree already.
expect fires "a defect after a backslash in a Go raw string" \
  'func normalize(p string) string { return strings.NewReplacer(`\`, `//`).Replace(p) + errFor("23505") }' \
  '23505'
# The same desync in the other direction disarms the waiver, which leaves the
# author of such a line no way through the gate at all.
expect silent "a waiver on a line holding a backslash raw string" \
  'var sepAndCode = strings.TrimSuffix(p, `\`) + "23505" // one-spelling-exempt: a build id, not a SQLSTATE'

echo
echo "== a block comment is code's absence, not a string's contents =="
# Both strip passes used to open a block comment on a bare `match(c, /\/\*/)`
# over the raw line, which a STRING is enough to forge. The block never closed,
# and every arm went blind from that line to the end of the file — no forgery
# needed, since a glob is spelt exactly like one.
expect fires "a /* inside a string does not blind the file" \
  'var globPattern = "**/*.go"

var dedupe = "23505"' \
  '23505'
expect fires "a /* inside a raw string does not blind the file" \
  'var globPattern = `**/*.go`

var dedupe = "23505"' \
  '23505'
# ...and the real thing still behaves: prose inside a block comment is not code,
# a waiver written in one still counts, and code after an inline one is judged.
expect silent "prose inside a real block comment" \
  '/*
A dedupe hit is "23505", named in sqlstate.go.
*/
func probe() {}'
expect silent "a waiver written in a block comment" \
  'var code = "23505" /* one-spelling-exempt: probing the gate */'
expect fires "code after an inline block comment" \
  'var n = 1 /* a note */ ; var dedupe = "23505"' \
  '23505'

# A backslash escapes the quote after it, so the string does NOT end there and
# the `//` inside it is not a comment. Drop that one character of lookahead and
# the scanner leaves the string early, reads the rest as a comment, and throws
# away the defect beside it.
expect fires "an escaped quote does not end the string early" \
  'var s = "a\"//b"; var dedupe = "23505"' \
  '23505'
# The scheme guard that used to sit in the comment test is gone; what actually
# spares a URL is the quote state, and this is the case that says so.
expect fires "a scheme inside a string is not a comment" \
  'var endpoint = "https://example.test/v1"; var dedupe = "23505"' \
  '23505'

# The gate's last line of defence, and the reason it has no residue paragraph:
# a file the scanner cannot follow to the end is a file it stopped reading, and
# reporting OK over one is the only failure a gate must never have. So it
# refuses by name instead. This case is the assertion firing.
expect unclosed "a file that ends inside an unclosed raw string" \
  'var query = `SELECT 1 FROM person'

echo
# A case whose own shell quoting is wrong prints an error and is SKIPPED, and
# the suite would go on to report OK over a case that never planted anything.
# The count is EXACT and not a floor: a floor lets probes disappear one at a
# time until only the floor's worth is left, which is the same silent shrinkage
# in slow motion. Derived from the file rather than typed, so adding a case
# cannot leave a stale number behind.
expected=$(grep -cE '^expect (fires|silent|unclosed) ' "$SELF")
# Only when nothing else failed. `ran` counts cases that passed, so a genuine
# assertion failure shortens it too — and reporting "some were skipped" on top
# of a real finding sends the reader after the wrong thing.
if [[ $fails -eq 0 && $ran -ne $expected ]]; then
  echo "FAIL: $ran cases ran but $expected are written — some were skipped before they planted anything"
  exit 1
fi
if [[ $fails -eq 1 ]]; then
  echo "FAIL: check-one-spelling.sh does not behave as its header claims"
  exit 1
fi
echo "OK: check-one-spelling.sh fires on each defect, stays silent on each lookalike"
