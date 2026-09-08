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

# The shape the flat listing got wrong. A merge brings in a side branch whose
# commits are OLDER than the reviewed one, so `rev-list`'s date order sorts them
# below it and slicing the list read them as already covered — silently, on
# exactly the pull requests complicated enough to need this report.
#
#   aaa1111 (reviewed) ... ccc3333 --- mmm4444 (merge)
#                          sss0001 --/          (older date, never reviewed)
merge_history=$'mmm4444 ccc3333 sss0001\nccc3333 bbb2222\nsss0001 aaa1111\nbbb2222 aaa1111\naaa1111 base000'
merge_out=""
merge_status=0
merge_out="$(REVIEW_COVERAGE_HEAD=mmm4444 REVIEW_COVERAGE_HISTORY="$merge_history" \
	REVIEW_COVERAGE_REVIEWS="[$(review cubic aaa1111 2026-09-01T10:00:00Z)]" "$report" 2>&1)" || merge_status=$?
if [[ "$merge_status" -ne 1 ]] || ! grep -qF "sss0001" <<<"$merge_out"; then
	echo "FAIL: a side branch merged in after the review was not reported — it is reachable from the tip and is not an ancestor of what was read, so a re-review has to cover it" >&2
	printf '%s\n' "$merge_out" >&2
	failures=$((failures + 1))
else
	echo "ok: a side branch older than the review is still reported as uncovered"
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

if [[ "$failures" -ne 0 ]]; then
	echo "FAIL: $failures case(s)" >&2
	exit 1
fi
echo "OK: test-review-coverage — every arm of the report, including the quiet ones"
