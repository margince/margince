#!/usr/bin/env bash
# test-check-review-coverage.sh — prove the coverage report still tells a review
# of the tip from a review of something older.
#
# It runs beside the reporter for the reason test-check-merge-verdict.sh runs
# beside the judge: this thing is quiet when it is working, so "no complaint"
# is exactly the signal that cannot be trusted on its own. The half of this
# suite that asserts exit 1 is what keeps the quiet half honest — a reporter
# rewritten to say less is one keystroke from saying nothing, and only a case
# that fails when it goes quiet can tell those apart.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
report="$root/scripts/check-review-coverage.sh"
failures=0

# The branch as `git rev-list --parents` hands it over: a commit and its
# parents, one per line. aaa1111 <- bbb2222 <- ccc3333, with aaa1111's own
# parent on the base and so outside the range.
history=$'ccc3333 bbb2222\nbbb2222 aaa1111\naaa1111 base000'

review() { printf '{"reviewer":"%s","sha":"%s","at":"%s"}' "$1" "$2" "$3"; }

# case_is <name> <reviews-json> <expected-exit> [substring the output must carry]
case_is() {
	local name="$1" reviews="$2" want="$3" must_say="${4:-}"
	local out status=0
	out="$(REVIEW_COVERAGE_HEAD=ccc3333 REVIEW_COVERAGE_HISTORY="$history" \
		REVIEW_COVERAGE_REVIEWS="$reviews" "$report" 2>&1)" || status=$?
	if [[ "$status" -ne "$want" ]]; then
		echo "FAIL: $name — expected exit $want, got $status" >&2
		printf '%s\n' "$out" >&2
		failures=$((failures + 1))
		return
	fi
	if [[ -n "$must_say" ]] && ! grep -qF -- "$must_say" <<<"$out"; then
		echo "FAIL: $name — exited $want but never said '$must_say'" >&2
		printf '%s\n' "$out" >&2
		failures=$((failures + 1))
		return
	fi
	echo "ok: $name"
}

# The shape the issue was filed about: reviewed, then fixed, then merged. The
# range it prints is what makes the finding actionable rather than a scolding.
case_is "a fix commit after the review is reported, by range" \
	"[$(review cubic aaa1111 2026-09-01T10:00:00Z)]" 1 "aaa1111..ccc3333"
case_is "and it names how many commits went unread" \
	"[$(review cubic aaa1111 2026-09-01T10:00:00Z)]" 1 "2 commit(s) landed after cubic"

# A reviewer who came back is covered by the later reading. Reporting the
# earlier one would ask for a re-review that already happened, which is how a
# report earns the mute it then never comes back from.
case_is "a second review of the tip clears the first" \
	"[$(review cubic aaa1111 2026-09-01T10:00:00Z),$(review cubic ccc3333 2026-09-01T12:00:00Z)]" 0 \
	"which is the tip"

# Per reviewer, not per pull request. One reviewer being current says nothing
# about another, and a report that took the newest record overall would let a
# fresh bot review paper over a human who read commit one.
case_is "one reviewer at the tip does not cover another who is behind" \
	"[$(review coderabbit ccc3333 2026-09-01T12:00:00Z),$(review human aaa1111 2026-09-01T10:00:00Z)]" 1 \
	"landed after human"

# A force-push destroys the evidence of what was read, and the approval goes on
# standing. This is the arm with no natural symptom: the reviewer looks current
# because a verdict is present.
case_is "a review naming a commit the branch no longer has is stale, not covered" \
	"[$(review cubic deadbee 2026-09-01T10:00:00Z)]" 1 "no longer has that commit"

# Every pull request is unreviewed for most of its life. Alarming here is how
# the report gets turned off before it ever catches anything.
case_is "a pull request nobody has reviewed says so and does not alarm" "[]" 0 \
	"nothing to be behind"
case_is "and it says that silence is not coverage" "[]" 0 \
	"invisible to it"

# Unset is a fetch that failed; `[]` is a pull request with no reviews. Reading
# the first as the second reports full coverage the day the API call breaks,
# which is the one direction with no red to notice.
unset_out=""
unset_status=0
# `env -u`, not merely leaving it out of the assignment list: a caller who
# exports REVIEW_COVERAGE_REVIEWS would otherwise have this case take the `[]`
# path and pass while proving nothing, which is the one arm whose whole subject
# is telling unset from empty.
unset_out="$(env -u REVIEW_COVERAGE_REVIEWS REVIEW_COVERAGE_HEAD=ccc3333 REVIEW_COVERAGE_HISTORY="$history" "$report" 2>&1)" || unset_status=$?
if [[ "$unset_status" -ne 2 ]] || ! grep -qF "cannot be told from a fetch that failed" <<<"$unset_out"; then
	echo "FAIL: unset reviews must refuse rather than read as no reviews — got exit $unset_status" >&2
	printf '%s\n' "$unset_out" >&2
	failures=$((failures + 1))
else
	echo "ok: an unset review list refuses rather than reading as no reviews"
fi

# The shape the flat listing got wrong. A merge brings in a side branch that
# diverged BEFORE the reviewed commit, so its commits are older and `rev-list`'s
# date order sorts them below the review point — slicing the list read them as
# already covered, silently, on exactly the pull requests complicated enough to
# need this report.
#
#   base000 --- aaa1111 (reviewed) --- bbb2222 --- ccc3333 --- mmm4444 (merge)
#           \-- sss0001 -------------------------------------/
#
# The side branch forks from base000, NOT from aaa1111: a child of the reviewed
# commit is structurally newer and would sort above it, which is the case the
# flat slice already got right. It is listed after aaa1111 below, because the
# order of these lines is what stands in for `rev-list`'s date order.
merge_history=$'mmm4444 ccc3333 sss0001\nccc3333 bbb2222\nbbb2222 aaa1111\naaa1111 base000\nsss0001 base000'
merge_out=""
merge_status=0
merge_out="$(REVIEW_COVERAGE_HEAD=mmm4444 REVIEW_COVERAGE_HISTORY="$merge_history" \
	REVIEW_COVERAGE_REVIEWS="[$(review cubic aaa1111 2026-09-01T10:00:00Z)]" "$report" 2>&1)" || merge_status=$?
if [[ "$merge_status" -ne 1 ]] || ! grep -qF "sss0001" <<<"$merge_out"; then
	echo "FAIL: a side branch that forked before the review was not reported — it is reachable from the tip and is not an ancestor of what was read, so a re-review has to cover it" >&2
	printf '%s\n' "$merge_out" >&2
	failures=$((failures + 1))
else
	echo "ok: a side branch that forked before the review is still reported as uncovered"
fi

# The other half of ancestry: the reviewed commit's OWN ancestors are covered,
# whatever their date. A walk that reported them would cry wolf on every merge.
covered_out=""
covered_status=0
covered_out="$(REVIEW_COVERAGE_HEAD=mmm4444 REVIEW_COVERAGE_HISTORY="$merge_history" \
	REVIEW_COVERAGE_REVIEWS="[$(review cubic mmm4444 2026-09-01T10:00:00Z)]" "$report" 2>&1)" || covered_status=$?
if [[ "$covered_status" -ne 0 ]] || ! grep -qF "which is the tip" <<<"$covered_out"; then
	echo "FAIL: a review of the merge itself was reported as behind — everything in the range is an ancestor of it" >&2
	printf '%s\n' "$covered_out" >&2
	failures=$((failures + 1))
else
	echo "ok: a review of the tip covers every ancestor the merge brought with it"
fi

# OLDEST FIRST, which the report's own prose promises and a reader relies on:
# the list reads in the order the commits were written, so the fix commit that
# needs re-reading is where you expect it. The graph walk produces no order of
# its own, so this is not a property that survives on its own.
order_out="$(REVIEW_COVERAGE_HEAD=ccc3333 REVIEW_COVERAGE_HISTORY="$history" \
	REVIEW_COVERAGE_REVIEWS="[$(review cubic aaa1111 2026-09-01T10:00:00Z)]" "$report" 2>&1 || true)"
if [[ "$(grep -cE '^(bbb2222|ccc3333)$' <<<"$order_out")" -ne 2 ]] ||
	[[ "$(grep -nE '^bbb2222$' <<<"$order_out" | cut -d: -f1)" -gt "$(grep -nE '^ccc3333$' <<<"$order_out" | cut -d: -f1)" ]]; then
	echo "FAIL: the unread commits were not listed oldest first" >&2
	printf '%s\n' "$order_out" >&2
	failures=$((failures + 1))
else
	echo "ok: the unread commits are listed oldest first"
fi

# A long branch is where the shape of the walk stops being an aesthetic
# question. Re-scanning the history per visited commit is quadratic AND spawns
# a process to do it, so the report times out and says nothing — which is worse
# than a wrong answer, because nothing is left to read.
# A long branch is where the shape of the walk stops being an aesthetic
# question. Re-scanning the history once per visited commit is quadratic AND
# spawns a process to do it, so a long-lived pull request gets no report at all
# rather than a wrong one — which is worse, because nothing is left to read.
#
# COUNTED, not timed. A wall-clock bound wide enough not to flake on a loaded
# runner is too wide to separate a quadratic walk from a linear one at any
# input size a suite can afford; and the property is not "fast", it is "the
# history is indexed once rather than re-read per commit". So the report runs
# with a counting shim ahead of awk on PATH, and the count is the assertion.
shim="$(mktemp -d)"
trap 'rm -rf "$shim"' EXIT
real_awk="$(command -v awk)"
cat >"$shim/awk" <<SHIM
#!/usr/bin/env bash
echo . >>"$shim/calls"
exec "$real_awk" "\$@"
SHIM
chmod +x "$shim/awk"
: >"$shim/calls"

# Built by awk rather than by appending in a loop: bash string concatenation is
# itself quadratic, and a fixture that cost more to build than to judge would
# be measuring the wrong thing. (Through the real awk, so it is not counted.)
long_history="$("$real_awk" 'BEGIN { for (i = 200; i >= 1; i--) printf "c%06d c%06d\n", i, i - 1 }')"
long_out="$(PATH="$shim:$PATH" REVIEW_COVERAGE_HEAD=c000200 REVIEW_COVERAGE_HISTORY="$long_history" \
	REVIEW_COVERAGE_REVIEWS="[$(review cubic c000001 2026-09-01T10:00:00Z)]" "$report" 2>&1 || true)"
awk_calls="$(grep -c . "$shim/calls" || true)"
if ! grep -qF "199 commit(s) landed after cubic" <<<"$long_out"; then
	echo "FAIL: a 200-commit branch was not counted correctly" >&2
	printf '%s\n' "$long_out" | head -3 >&2
	failures=$((failures + 1))
elif (( awk_calls > 10 )); then
	echo "FAIL: the report read the history $awk_calls times for a 200-commit branch — it is re-reading per commit, so a long-lived pull request will get no report at all" >&2
	failures=$((failures + 1))
else
	echo "ok: a 200-commit branch is judged in $awk_calls pass(es) over the history"
fi

if [[ "$failures" -ne 0 ]]; then
	echo "FAIL: $failures case(s)" >&2
	exit 1
fi
echo "OK: test-review-coverage — every arm of the report, including the quiet ones"
