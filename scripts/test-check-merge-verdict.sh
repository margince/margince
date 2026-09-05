#!/usr/bin/env bash
# test-check-merge-verdict.sh — prove the merge alarm still tells an absent
# verdict from a green one.
#
# It runs beside the judge for the reason `test-ci-verdict.sh` runs beside the
# verdict: this thing is silent when it is working, so "the alarm did not go
# off" is exactly the signal that was already untrustworthy. The case that
# matters most is the check that is MISSING rather than red — #2504's own shape,
# and the one an "is it green?" question cannot see, because jq reading an
# absent field and jq reading a passing one both answer with something falsy if
# the script is written carelessly.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
judge="$root/scripts/check-merge-verdict.sh"
failures=0

# case_is <name> <pulls-json> <checks-json> <expected-exit> [substring the output must carry]
case_is() {
	local name="$1" pulls="$2" checks="$3" want="$4" must_say="${5:-}"
	local out status=0
	out="$(MERGE_VERDICT_SHA=abc1234 MERGE_VERDICT_PULLS="$pulls" MERGE_VERDICT_CHECKS="$checks" \
		"$judge" 2>&1)" || status=$?
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

merged='2026-08-24T02:48:19Z'
pr="[{\"number\":2516,\"merged_at\":\"$merged\"}]"
check() { printf '{"check_runs":[{"name":"ci","status":"completed","conclusion":"%s","completed_at":"%s"}]}' "$1" "$2"; }

# The shape every well-behaved merge has, and the one every case below is a
# departure from. Without it passing, a suite of failures proves only that the
# script exits non-zero.
case_is "green before the merge" "$pr" "$(check success 2026-08-24T02:29:56Z)" 0 "#2516"

# #2504's four merges. An absent verdict is not a red one, and nothing that asks
# "was it green?" can see the difference.
case_is "the required check never reported" "$pr" '{"check_runs":[]}' 1 "before its required \`ci\` check reported at all"
case_is "some other check reported, ci did not" "$pr" \
	'{"check_runs":[{"name":"deterministic-gates","status":"completed","conclusion":"success","completed_at":"2026-08-24T02:16:58Z"}]}' \
	1 "before its required \`ci\` check reported at all"

case_is "merged over a red ci" "$pr" "$(check failure 2026-08-24T02:29:56Z)" 1 'was `failure`'
case_is "merged over a cancelled ci" "$pr" "$(check cancelled 2026-08-24T02:29:56Z)" 1 'was `cancelled`'
# GitHub counts a skipped required check as passing. This does not.
case_is "merged over a skipped ci" "$pr" "$(check skipped 2026-08-24T02:29:56Z)" 1 'was `skipped`'
case_is "ci still running at merge time" "$pr" \
	'{"check_runs":[{"name":"ci","status":"in_progress","conclusion":null}]}' 1 'was `still running`'

# Green, but only afterwards. The merge was decided on an unfinished run and the
# answer arriving later is luck, not a gate — invisible to anything that reads
# the conclusion alone.
case_is "green AFTER the merge landed" "$pr" "$(check success 2026-08-24T03:10:00Z)" 1 "before its required \`ci\` check finished"

# A re-run after the merge must not clear the record of the merge it was absent
# for, so the OLDEST run is the one judged.
case_is "a later re-run does not overwrite the verdict at merge time" "$pr" \
	'{"check_runs":[{"name":"ci","status":"completed","conclusion":"failure","completed_at":"2026-08-24T02:29:56Z"},{"name":"ci","status":"completed","conclusion":"success","completed_at":"2026-08-25T09:00:00Z"}]}' \
	1 'was `failure`'

case_is "no pull request at all" '[]' '{"check_runs":[]}' 1 "no pull request naming it"

# A commit with no pull request and a lookup that did not happen are different
# facts, and reporting the second as the first is how an alarm gets switched
# off. Exit 2, not 1: this is the script failing, not a finding.
out=''
status=0
out="$(MERGE_VERDICT_SHA=abc1234 MERGE_VERDICT_PULLS='' "$judge" 2>&1)" || status=$?
if [[ "$status" -ne 2 ]] || ! grep -qF "the lookup did not happen" <<<"$out"; then
	echo "FAIL: an unset lookup is reported as a finding rather than as a broken alarm" >&2
	printf '%s\n' "$out" >&2
	failures=$((failures + 1))
else
	echo "ok: an unset lookup fails the alarm rather than accusing the merge"
fi

status=0
out="$(MERGE_VERDICT_PULLS='[]' "$judge" 2>&1)" || status=$?
if [[ "$status" -ne 2 ]]; then
	echo "FAIL: a missing commit sha is judged rather than refused" >&2
	failures=$((failures + 1))
else
	echo "ok: a missing commit sha is refused"
fi

if [[ "$failures" -ne 0 ]]; then
	echo "test-check-merge-verdict: $failures case(s) failed" >&2
	exit 1
fi
echo "merge verdict alarm: every case holds"
