#!/usr/bin/env bash
# Prove check-money-scale.sh fires in BOTH languages, honours its waiver, and
# does not read a comment as code.
#
# Probes live outside the repository and the gate is pointed at them, so a
# planted defect can never collide with a concurrent `make -j` job.
set -uo pipefail
# Resolved BEFORE the cd. `$0` is whatever the caller typed, so after changing
# directory it can name a path that does not exist — and the census below then
# reads zero cases and reports a fully passing run as a failure. Launching the
# suite from `scripts/` did exactly that.
SELF="$(CDPATH= cd -P -- "$(dirname -- "$0")" && pwd)/$(basename -- "$0")"
cd "$(dirname "$0")/.."

GATE=./scripts/check-money-scale.sh
PROBE="$(mktemp -d)"
trap 'rm -rf "$PROBE"' EXIT
fails=0
ran=0

# expect <fires|silent> <go|ts> <name> <body>
expect() {
  local want="$1" lang="$2" name="$3" body="$4" out rc file
  rm -f "$PROBE"/probe.*
  if [[ "$lang" == go ]]; then
    file="$PROBE/probe.go"
    printf '// SPDX-License-Identifier: BUSL-1.1\npackage probe\n\n%s\n' "$body" > "$file"
  else
    file="$PROBE/probe.ts"
    printf '%s\n' "$body" > "$file"
  fi
  out="$(MONEY_SCALE_GO_SCAN="$PROBE" MONEY_SCALE_TS_SCAN="$PROBE" $GATE 2>&1)"; rc=$?
  rm -f "$file"
  # A non-zero exit is not the same as a DETECTION. The gate also exits 1 when
  # a scan root is missing or the script itself errors, and a case that reads
  # only the status would score that as the defect being caught — a probe
  # passing for a reason that has nothing to do with what it plants.
  if [[ "$want" == fires ]] && ! grep -q "scaled by a hard-coded power of ten" <<< "$out"; then
    echo "FAIL: $name — the gate exited $rc without reporting a scale finding, so this proves nothing"
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
  if [[ "$want" == unclosed ]] && ! grep -qF -- "$file" <<< "$out"; then
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
echo "== Go: the shape that shipped four times =="
expect fires go "a divide"   'func probe(amountMinor int64) int64 { return amountMinor / 100 }'
expect fires go "a multiply, wrapped by the formatter" 'func probe(major int64) int64 {
	amountMinor := int64(
		major * 100,
	)
	return amountMinor
}'
expect fires go "a remainder for the cents half" 'func probe(minor int64) int64 { return minor % 100 }'

# Every power the detector claims, not just the euro's. A three-decimal
# currency scales by 1000 and a one-decimal by 10, and a probe set that only
# ever plants 100 would not notice the detector losing either.
expect fires go "a KWD-style 1000"     'func probe(amountMinor int64) int64 { return amountMinor / 1000 }'
expect fires go "a one-digit 10"       'func probe(amountMinor int64) int64 { return amountMinor / 10 }'
expect fires go "a four-zero 10000"    'func probe(amountMinor int64) int64 { return amountMinor / 10000 }'
expect fires ts "a KWD-style 1000"     'export const toWire = (amount_minor: number) => amount_minor / 1000;'
expect fires ts "a one-digit 10"       'export const toWire = (amount_minor: number) => amount_minor * 10;'
expect fires ts "a remainder for the fractional half" \
  'export const cents = (valueMinor: number) => valueMinor % 100;'

# The identifier may be a Go or SQL-style constant.
expect fires go "an upper-case identifier" 'const AMOUNT_MINOR_SCALE = 1
func probe(v int64) int64 { return AMOUNT_MINOR_SCALE * v / 100 }'

echo
echo "== TypeScript: the write direction the Go-only gate could not see =="
# The wrap biome actually produces for a longer expression: five lines, which
# the first four-line bound flushed in half.
expect fires ts "a multiply wrapped across five lines" 'export const toWire = (amount: string) => {
  const amountMinor =
    Math.round(
      Number(
        amount,
      ) * 100,
    );
  return amountMinor;
};'

# A `//` inside a TS regex literal is not a comment, and reading it as one
# truncated the real defect after it off the line.
expect fires ts "a regex literal does not hide the defect after it" \
  'export const f = (u: string, amountMinor: number) => { const clean = u.replace(/^https?:\/\//, ""); return [clean, amountMinor / 100]; };'

# Grouped integer literals are the house style for a four-digit scale, and the
# blanket strip that spared the basis-point divisor was hiding them.
expect fires ts "a grouped 1_000"  'export const m = (kwdMinor: number) => kwdMinor / 1_000;'
expect fires go "a grouped 10_000" 'func probe(amountMinor int64) int64 { return amountMinor / 10_000 }'

# A `${…}` on a continuation line used to reset the statement accumulator to
# zero, because blankStrings tracked its braces in a variable it had forgotten
# to declare local — and awk has no other way to declare one, so it was the
# SAME global the strip pass counts brackets in. The buffer flushed mid-
# statement and the two halves of the defect were judged apart.
expect fires ts "a wrapped defect after a template interpolation" 'const amountMinor = toMinor(
  `${label}`,
  major * 100,
);'
# ...and the inverse: an interpolation must not GLUE unrelated statements
# either, which the same collision did in the other direction.
expect silent ts "an open interpolation does not glue later statements" 'const s = `${
  x}`;
const valueMinor = 1;
const ageMs = 2 * 100;'

# The same two library holes on the money side: a backslash inside a Go raw
# string blanked the rest of the line as string content, and a `/*` inside a
# string opened a block comment that never closed.
expect fires go "a defect after a backslash in a Go raw string" 'func windows(path string, amountMinor int64) string {
	return strings.TrimPrefix(path, `\`) + fmt.Sprint(amountMinor/100)
}'
expect fires go "a defect after a /* inside a string" 'var globPattern = "**/*.go"

func probe(amountMinor int64) int64 { return amountMinor / 100 }'

# The same one character of lookahead, on the blanking side. Without it the
# scanner leaves the string at the escaped quote and reads the PROSE after it
# as code — so a line explaining the defect becomes the defect.
expect silent ts "prose quoting the shape, after an escaped quote" 'const help = "write \"amountMinor / 100\" instead";'

# A Go raw string spans lines, and its interior is string content on EVERY one
# of them. Blanking only the line the backtick opens on leaves the rest read as
# code, so a query that mentions the shape becomes the shape.
expect silent go "the shape inside a multi-line raw string" 'const explain = `the old code did
	amountMinor / 100
and that was the bug`'

# TypeScript has three string delimiters and the scanner has to honour all
# three. A single-quoted path carrying a `//` used to end the line there and
# throw the defect after it away...
expect fires ts "a  //  inside a single-quoted string" "const path = '/oauth // token'; const amountMinor = major * 100;"
# ...and a template literal's contents are contents, so a line quoting the
# shape inside one is prose, not arithmetic.
expect silent ts "the shape inside a template literal" 'const help = `amountMinor / 100`;'

# The same last line of defence. A backtick inside a regex literal opens a
# template literal the language never closes, so the scanner runs off the end
# of the file — and the run says which file rather than saying OK.
expect unclosed ts "a file that ends inside an unclosed template literal" 'const re = /[`]/;
const amountMinor = major * 100;'

echo
echo "== what review found in the shared scanner, each direction =="
# A Go backtick string is RAW and never interpolates, so `${…}` written inside
# one is prose. Keeping it as executable text reported a comment describing the
# old bug as the bug.
expect silent go 'an interpolation inside a Go raw string is prose' 'const explain = `see ${amountMinor / 100} in the old code`'
# In TypeScript it IS executable, on one line and across several.
expect fires ts "a defect inside an interpolation" 'const s = `${amountMinor / 100}`;'
expect fires ts "a defect inside a multi-line interpolation" 'const s = `${
  amountMinor / 100
}`;'
# A remainder is how the cents half is taken, and it was the one shape in this
# gate's own vocabulary its continuation rule could not rejoin.
expect fires ts "a remainder split after a trailing %" 'const centsPart = amountMinor %
  100;'
# A comment-only line is not a statement boundary; treating it as one judged
# the two halves of a wrapped expression apart.
expect fires ts "a comment line between a name and its power" 'const amountMinor =
  /* a note */
  major * 100;'
# A line ending ON a colon has its value on the next one — how biome wraps a
# long object property, and the write direction this gate exists for.
expect fires ts "an object property breaking after the colon" 'const body = {
  amountMinor:
    major * 100,
};'
# ...while a member ending in a COMMA still ends its statement, which is the
# case the old rule was written for and got right.
expect silent ts "a comma-terminated member near an unrelated power" 'const row = {
  valueMinor: 1,
  label: "x",
  ageMs: seconds * 1000,
};'

# Inside an interpolation the contents are CODE, so a `//` there is a real
# comment — and the interpolation is still open after it. Zeroing that state
# read the closing `}` as ordinary code and the backtick after it as a NEW
# template, leaving the scanner inside a string for the rest of the file.
expect silent ts "a // inside an interpolation is a comment" 'const s = `${x // amountMinor / 100
}`;'
expect fires ts "and the interpolation still closes afterwards" 'const s = `${x // a note
}`;
const amountMinor = major * 100;'
# A nested string inside an interpolation is still a string.
expect silent ts "a nested string inside an interpolation" 'const s = `${label("see amountMinor / 100 here")}`;'
expect fires ts "a defect beside that nested string" 'const s = `${label("a note") + amountMinor / 100}`;'
# A statement buffered at end of file must be reported under ITS file, not the
# next one, and must not be joined to that file's first line.
expect silent ts "a dangling continuation at end of file" 'const amountMinor ='

# The contexts NEST, and a pair of counters cannot say so. `${`x`}` opens a
# template, an interpolation and a second template; a flat "am I in a template"
# flag was overwritten by the inner one and its contents read as code.
expect silent ts "a nested template inside an interpolation" 'const s = `${`amountMinor / 100`}`;'
expect fires ts "a defect past a nested template" 'const s = `${`x` + amountMinor / 100}`;'
# A `case x:` or a bare label ends with a colon and does NOT continue — its body
# is a separate statement, and joining it paired a `case valueMinor:` with an
# unrelated `ratio * 100` in the arm below.
expect silent ts "a case label over unrelated arithmetic" 'switch (k) {
  case valueMinor:
    widthPct = ratio * 100;
}'
expect silent ts "a default arm over unrelated arithmetic" 'switch (k) {
  default:
    widthPct = ratio * 100;
}'
expect fires ts "a case arm that IS a defect" 'switch (k) {
  case "eur":
    amountMinor = major * 100;
}'
# A label may span an open bracket, and its closing `):` is still the label's
# end rather than a continuation into the arm.
expect silent ts "a case label spanning a bracket" 'switch (k) {
  case pick(
    valueMinor):
    widthPct = ratio * 100;
}'
# A label with code ON it is still just a label line. This is here because the
# first version of the rule carried "am I in a label" across lines, and a label
# like this never cleared it — so every later line in the file was flushed on
# its own and nothing below the switch was judged at all.
expect fires ts "code on the label line, defect below the switch" 'switch (k) {
  case "eur": doThing();
}
const amountMinor =
  major * 100;'
# A quoted string cannot legally span a line, so it does not carry — carrying it
# would blind the rest of the FILE over a typo, which is the one direction a
# scanner must not fail in.
expect fires ts "an unterminated quote does not blind the next line" 'const broken = "oops;
const amountMinor = major * 100;'

expect fires ts "a multiply on the write path, wrapped" 'export const toWire = (amount: string) => ({
  amount_minor: Math.round(Number(amount) * 100),
});'
expect fires ts "a divide on the read path" 'export const seed = (valueMinor: number) => valueMinor / 100;'

echo
echo "== a lookalike that is not money =="
# The anchor is the identifier, so arithmetic on 100 with no minor amount near
# it is untouched — otherwise every percentage in the tree would be a finding.
# Over-matching is the other way a scanner stops being read. A grouped literal
# larger than any minor unit is not a scale, and prose describing the defect is
# not the defect.
expect silent ts "a grouped literal past any minor unit" \
  'export const m = (amountMinor: number) => amountMinor / 1_000_000;'
# A template interpolation is executable, not string content. Blanking it hid
# the conversion inside entirely.
expect fires ts "a conversion inside a template interpolation" \
  'export const s = (amountMinor: number) => `total: ${amountMinor / 100}`;'

expect silent ts "the shape mentioned inside a string" \
  'export const note = "see amountMinor / 100 in the old code";'
expect silent go "the shape mentioned inside a Go string" \
  'const note = "amountMinor / 100 was the old spelling"'

expect silent go "a percentage"     'func probe(part, whole int64) int64 { return part * 100 / whole }'
expect silent ts "a progress width" 'export const pct = (done: number, total: number) => (done / total) * 100;'

echo
echo "== the waiver, and comments =="
expect silent go "a waived line" \
  'func probe(amountMinor int64) int64 { return amountMinor / 100 } // money-scale-exempt: probing the gate'
# The marker must be in a REAL comment. A line carrying it inside a STRING was
# waiving itself, so any arithmetic sharing that line bypassed the gate under a
# marker nobody wrote as one — probed, and it did.
expect fires go "a marker inside a string waives nothing" \
  'func probe(amountMinor int64) (int64, string) { return amountMinor / 100, "// money-scale-exempt: fake" }'
expect fires ts "a marker inside a TS string waives nothing" \
  'export const toWire = (amount_minor: number) => [amount_minor / 100, "// money-scale-exempt: fake"];'

expect fires go "a waiver silences its own line only" \
  'func waived(amountMinor int64) int64 { return amountMinor / 100 } // money-scale-exempt: probing the gate
func notWaived(valueMinor int64) int64 { return valueMinor / 100 }'
expect silent go "the shape described in a line comment" \
  '// The old spelling was amountMinor / 100, which VND does not survive.'
expect silent go "the shape described in a block comment" \
  '/*
The old spelling was amountMinor / 100.
*/'
expect silent ts "the shape described in a TS comment" \
  '// Not `(valueMinor / 100).toFixed(2)` — the scale is the currency'"'"'s.'

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
  echo "FAIL: check-money-scale.sh does not behave as its header claims"
  exit 1
fi
echo "OK: check-money-scale.sh fires in both languages, and only on money"
