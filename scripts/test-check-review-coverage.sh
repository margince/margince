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

# The branch, newest first, as `git rev-list` hands it over.
history=$'ccc3333\nbbb2222\naaa1111'

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
unset_out="$(REVIEW_COVERAGE_HEAD=ccc3333 REVIEW_COVERAGE_HISTORY="$history" "$report" 2>&1)" || unset_status=$?
if [[ "$unset_status" -ne 2 ]] || ! grep -qF "cannot be told from a fetch that failed" <<<"$unset_out"; then
	echo "FAIL: unset reviews must refuse rather than read as no reviews — got exit $unset_status" >&2
	printf '%s\n' "$unset_out" >&2
	failures=$((failures + 1))
else
	echo "ok: an unset review list refuses rather than reading as no reviews"
fi

if [[ "$failures" -ne 0 ]]; then
	echo "FAIL: $failures case(s)" >&2
	exit 1
fi
echo "OK: test-review-coverage — every arm of the report, including the quiet ones"
