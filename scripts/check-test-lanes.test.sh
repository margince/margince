#!/usr/bin/env bash
# The lane gate's own test — it gates what the gate READS, not its verdict.
#
# check-test-lanes.sh answers about a file's text, and the interesting half of
# that answer is what it declines to see. A version that reported a comment was
# shipped: `backend/laneconnbudget_test.go` explained why a term in its
# arithmetic is what it is, said "a single pgx.Connect" in a sentence, and was
# reported as opening a connection. The rule was right and the reading was
# wrong, and what a gate like that produces is not a fix — it is an author
# rewording a true comment to avoid a term, and the next reader learning that
# naming the thing is what is forbidden.
#
# So each case below is a FILE the gate has to judge, planted in a throwaway
# tree, and each states the reason it must be judged that way. The two halves
# are equally load-bearing: a stripper that dropped code would make this gate
# green over every violation, so every "must be reported" case is as much a
# test of the stripper as the "must not" ones are.
#
# Usage: bash scripts/check-test-lanes.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATE="$SCRIPT_DIR/check-test-lanes.sh"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

# The gate walks `backend` relative to its own parent, so the fixture tree is
# built as a whole repository root with the gate copied into it. Copied rather
# than invoked in place: pointing the real gate at a fixture would have it read
# this repository's own backend as well, and a violation there would be
# indistinguishable from the planted one.
plant() {
  local name="$1" body="$2"
  rm -rf "$TMP/repo"
  mkdir -p "$TMP/repo/scripts" "$TMP/repo/backend"
  cp "$GATE" "$TMP/repo/scripts/check-test-lanes.sh"
  cp "$SCRIPT_DIR/lib-strip-go-comments.awk" "$TMP/repo/scripts/"
  printf '%s\n' "$body" > "$TMP/repo/backend/${name}_test.go"
}

# reported runs the gate over the planted tree and answers whether it named the
# file.
reported() {
  ! ( cd "$TMP/repo" && ./scripts/check-test-lanes.sh >/dev/null 2>&1 )
}

# --- must NOT be reported: the marker is prose ------------------------------

plant lineComment 'package x

// db verbs are a single pgx.Connect, and at a handover JOBS of them overlap.
func TestArithmetic(t *testing.T) {}'
if reported; then
  fail "a marker inside a // comment was reported — the gate reads prose as code, which is what forces an author to reword a true sentence"
fi

plant blockComment 'package x

/*
Two lines of explanation, one of which says pgxpool.New for a reason.
*/
func TestArithmetic(t *testing.T) {}'
if reported; then
  fail "a marker inside a /* */ comment was reported"
fi

plant trailingComment 'package x

func TestArithmetic(t *testing.T) {
	budget := 24 // the ceiling a pgx.Connect would spend
	_ = budget
}'
if reported; then
  fail "a marker in a trailing comment was reported"
fi

# --- must BE reported: the marker is code ----------------------------------

plant realConnect 'package x

func TestOpensAConnection(t *testing.T) {
	conn, _ := pgx.Connect(ctx, dsn)
	_ = conn
}'
if ! reported; then
  fail "a real pgx.Connect was NOT reported — the stripper is eating code, and this gate is green over every violation it exists to catch"
fi

plant realEnv 'package x

func TestReadsTheHarnessDSN(t *testing.T) {
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	_ = dsn
}'
if ! reported; then
  fail "a unit test reaching for MARGINCE_TEST_DSN was NOT reported"
fi

# A URL inside a string carries `//`, and the marker comes AFTER it ON THE SAME
# LINE. That ordering is the whole case: a stripper that cuts from the first
# `//` it sees drops the rest of this line, the marker with it, and the gate
# reads a file that opens a connection as one that does not. On separate lines
# the same defect is invisible, which is why they are together here.
plant urlBeforeMarker 'package x

func TestDialsWithAURL(t *testing.T) {
	cfg := map[string]string{"docs": "https://example.test/path", "dsn": os.Getenv("MARGINCE_TEST_DSN")}
	_ = cfg
}'
if ! reported; then
  fail "a marker following a // inside a string literal on the same line was NOT reported — the stripper is cutting at a // it should not have seen"
fi

# The marker inside a string literal IS the violation's usual shape, so a
# stripper that spared strings would spare it.
plant markerInString 'package x

func TestBuildsADSN(t *testing.T) {
	_ = os.Getenv("MARGINCE_TEST_APP_DSN")
}'
if ! reported; then
  fail "a marker inside a string literal was NOT reported — strings are code here, not prose"
fi

# --- the lane tag still exempts a file -------------------------------------

plant tagged '//go:build integration

package x

func TestOpensAConnection(t *testing.T) {
	conn, _ := pgx.Connect(ctx, dsn)
	_ = conn
}'
if reported; then
  fail "a file carrying //go:build integration was reported — the tag is what the whole rule is stated in terms of"
fi

if [[ "$FAILURES" -ne 0 ]]; then
  echo "" >&2
  echo "$FAILURES case(s) failed." >&2
  exit 1
fi
echo "==> test-lanes census: comments are prose, strings and calls are code (8 cases)"
