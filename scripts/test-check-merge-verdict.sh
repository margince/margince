#!/usr/bin/env bash
# test-check-merge-verdict.sh — prove the merge alarm still tells an ADVERSE
# verdict from an absent one.
#
# It runs beside the judge for the reason `test-ci-verdict.sh` runs beside the
# verdict: this thing is silent when it is working, so "the alarm did not go
# off" is exactly the signal that was already untrustworthy. Silence is now the
# answer to far more inputs than it used to be — every absent verdict — which
# makes the half of this suite that asserts exit 0 the half that matters. An
# alarm rewritten to say less is one keystroke from saying nothing, and only a
# case that FAILS when it goes quiet can tell those apart.
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

# THE ONE FINDING THAT REMAINS about a check: it reported, and the answer was
# adverse. Merging past `ci` is a standing decision, so the alarm is silent about
# a verdict that is merely absent — but never about one that came back bad.
case_is "the required check reported red" "$pr" "$(check failure 2026-08-24T02:29:56Z)" 1 'reported `failure`'
case_is "the required check timed out" "$pr" "$(check timed_out 2026-08-24T02:29:56Z)" 1 'reported `timed_out`'

# Green, and green only afterwards. The merge was decided on an unfinished run;
# under a bypass that is the expected shape, not a finding.
case_is "green AFTER the merge landed" "$pr" "$(check success 2026-08-24T03:10:00Z)" 0 "merged behind a green"

# THE SILENT ARMS. Each is a way to have NO answer, and each must exit 0 — but
# each is asserted on its message too, so a judge that has stopped reading the
# verdict at all cannot pass this by falling through to a bare exit 0.
case_is "the required check never reported" "$pr" '{"check_runs":[]}' 0 "no \`ci\` verdict to read"
case_is "some other check reported, ci did not" "$pr" \
	'{"check_runs":[{"name":"deterministic-gates","status":"completed","conclusion":"success","completed_at":"2026-08-24T02:16:58Z"}]}' \
	0 "no \`ci\` verdict to read"
# Deleting the branch on merge cancels what was still running on it. That is the
# tidy-up, not the tree.
case_is "the required check was cancelled" "$pr" "$(check cancelled 2026-08-24T02:29:56Z)" 0 'is an absent answer'
case_is "the required check was skipped" "$pr" "$(check skipped 2026-08-24T02:29:56Z)" 0 'is an absent answer'
case_is "ci still running when read" "$pr" \
	'{"check_runs":[{"name":"ci","status":"in_progress","conclusion":null}]}' 0 'is an absent answer'

# A re-run after the merge must not clear the record of the merge it was absent
# for, so the OLDEST run is the one judged.
case_is "a later re-run does not overwrite the verdict at merge time" "$pr" \
	'{"check_runs":[{"name":"ci","status":"completed","conclusion":"failure","completed_at":"2026-08-24T02:29:56Z"},{"name":"ci","status":"completed","conclusion":"success","completed_at":"2026-08-25T09:00:00Z"}]}' \
	1 'reported `failure`'

# The other finding, and the loudest: nothing reviewed this at all.
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
