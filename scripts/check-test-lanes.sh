#!/usr/bin/env bash
# Test-lane separation.
#
# A unit test (one WITHOUT a `//go:build integration` or `//go:build livesmoke`
# tag, so it runs under `make test`) must never open a REAL Postgres/Redis
# connection. Anything that needs real infrastructure belongs in the
# integration lane. This keeps `make test` hermetic and kills the "DB test
# that silently degrades in the unit lane" anti-pattern.
#
# Fakes are fine and NOT flagged: in-memory fake sql drivers carry none of
# the markers below. If a miniredis-style fake (which does dial via
# redis.NewClient) ever joins the unit lane, narrow that marker then —
# deliberately, in this file — rather than pre-weakening the gate today.
set -euo pipefail
cd "$(dirname "$0")/.."

# Markers that only a real connection uses. MARGINCE_TEST_* are the env vars
# the integration harness reads (backend/Makefile exports them from db-up's
# port contract); a unit test reaching for them is in the wrong lane.
real='sql\.Open\("(postgres|pgx)"|pgxpool\.New|pgx\.Connect|os\.Getenv\("MARGINCE_TEST_(DSN|APP_DSN|REDIS)"\)|redis\.ParseURL|redis\.New(Universal)?Client'

# CODE, not prose. The markers above are grepped out of the file's TEXT, and a
# _test.go that merely NAMES one while explaining something is not opening a
# connection: "a single pgx.Connect" in a sentence about arithmetic was reported
# as a violation of a rule that file keeps. The workaround such a gate forces is
# worse than the miss — an author rewords a true comment to avoid a term, and
# the next reader learns that naming the thing is what is forbidden.
#
# So comments are stripped before the match. String literals are NOT, and that
# is the line: a DSN in a string is exactly the shape this gate looks for, and a
# stripper that spared strings would spare the violation.
# The stripper is its own file: an awk program inlined into a shell string has
# to survive two levels of quoting, and the version that did not was silently
# empty — which made this gate print OK over a tree it had not read. A file is
# read by awk itself, and its own test plants a violation to prove it.
STRIP_AWK="$(dirname "$0")/lib-strip-go-comments.awk"

# lane_report prints one file's violation report, or nothing when the file is
# clean.
lane_report() {
  local f="$1" code lines ln
  # Skip files already in a non-unit lane. Build constraints sit anywhere
  # above the package clause, so scan up to it instead of a fixed head.
  if sed -n '/^package /q;p' "$f" | grep -qE '^//go:build .*(integration|livesmoke)'; then
    return 0
  fi
  # A stripper that fails prints nothing, and nothing matches nothing, so its
  # failure must stop the gate rather than read as a clean file.
  code="$(awk -f "$STRIP_AWK" "$f")" || { echo "test-lanes: cannot strip comments from $f" >&2; return 2; }
  grep -Eq "$real" <<<"$code" || return 0
  # Reported against the ORIGINAL line, so the number is the one an author
  # opens and the text is what they wrote — the stripped copy only decides
  # WHICH lines are reported.
  lines="VIOLATION (unit test opens real infra — add //go:build integration, or fake the boundary): $f"
  while IFS= read -r ln; do
    lines+=$'\n'"    $ln:$(sed -n "${ln}p" "$f")"
  done < <(printf '%s\n' "$code" | grep -nE "$real" | cut -d: -f1)
  printf '%s\n' "$lines"
}
export -f lane_report
export real STRIP_AWK

# Each file is independent, so workers check one batch each, side by side. A
# batch writes to its own file: one report can outgrow an atomic pipe write.
jobs="$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)"
reports="$(mktemp -d)"
trap 'rm -rf "$reports"' EXIT
export reports
# Search roots: the backend hand-written Go tree, and nothing else is in scope.
status=0
find backend -name '*_test.go' -print0 2>/dev/null | sort -z \
  | xargs -0 -P "$jobs" -n 64 bash -c 'set -uo pipefail; rc=0
      out="$reports/${1//\//_}"
      for f; do lane_report "$f" >>"$out" || rc=$?; done; exit "$rc"' lane_report || status=$?
report="$(find "$reports" -type f | sort | xargs cat)"
if [[ -n "$report" ]]; then
  printf '%s\n' "$report"
fi

if [[ -n "$report" || "$status" -ne 0 ]]; then
  echo "FAIL: test-lanes — real-infra tests must carry //go:build integration."
  exit 1
fi
echo "OK: test-lanes — no unit test opens a real DB/Redis"
