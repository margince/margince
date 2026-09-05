#!/usr/bin/env bash
# check-merge-verdict.sh — did what just landed on main have a verdict behind it?
#
# `ci` is the one required check, and a repository role can merge past it. That
# is deliberate and stays: an infrastructure outage that reds every pull
# request, a revert that has to land now. What was missing is that a merge which
# used it looked exactly like one that did not. The pull request closes
# green-looking in the list, `main-health` runs two-hourly and reports the last
# green it saw, and the next signal anybody gets is somebody ELSE's pull request
# going red against a base they did not break — which is how #2504's four
# merges were found, twice, independently, each time costing a rebase and a full
# local lane to rule the finder's own diff out.
#
# So this does not prevent anything. It makes the fact loud, at push time,
# attributed. Prevention is a branch-protection decision and is not this
# script's to make.
#
# WHAT IT REPORTS, and each is a distinct failure that reads the same from
# outside:
#
#   - nothing merged it. No pull request names this commit, so no review and no
#     check ever applied to it.
#   - the check reported, and the verdict was adverse. The tree that landed does
#     not pass its own required check.
#
# WHAT IT DELIBERATELY DOES NOT REPORT: an ABSENT verdict. Merging past `ci` is a
# standing decision here, not an incident, and `ci` is a fan-in that only starts
# once every lane has finished — so it posts roughly ten minutes after a run
# begins and a merge inside that window sees nothing. Reporting that was
# reporting the decision back to the people who made it: seventeen issues in
# under five hours, every one of them describing a bypass working as intended.
# An alarm that fires on the expected state is an alarm that gets turned off,
# and it would have buried the two findings above.
#
# Reads its evidence from the environment rather than fetching it, so every arm
# above is drivable from a fixture — the same shape ci-verdict.sh uses, and for
# the same reason: a reporter that can only be observed exiting 0 is not
# evidence of anything.
set -euo pipefail

sha="${MERGE_VERDICT_SHA:-}"
pulls="${MERGE_VERDICT_PULLS:-}"
checks="${MERGE_VERDICT_CHECKS:-[]}"
required="${MERGE_VERDICT_REQUIRED:-ci}"

if [[ -z "$sha" ]]; then
	echo "FAIL: MERGE_VERDICT_SHA is empty — there is no commit to ask about. Pass \${{ github.sha }}." >&2
	exit 2
fi

# Empty is not the same as `[]`. A fetch that failed and a commit with no pull
# request both leave this unset if the caller is careless, and treating the
# first as the second would report every merge as unreviewed the day the API
# call breaks — a gate that cries wolf is turned off, which costs more than it
# ever caught.
if [[ -z "$pulls" ]]; then
	echo "FAIL: MERGE_VERDICT_PULLS is empty. A commit with no pull request is the JSON array \`[]\`; empty means the lookup did not happen, and this cannot tell those apart." >&2
	exit 2
fi

emit() {
	local why="$1"
	echo "$why"
	if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
		# Delimited rather than `key=value`. Every reason this writes is one
		# line today, and the day one is not, the plain form would end the
		# output file mid-sentence and take the following key with it.
		{
			echo "why<<MERGE_VERDICT_EOF"
			echo "$why"
			echo "MERGE_VERDICT_EOF"
			echo "pr=${pr_number:-}"
		} >>"$GITHUB_OUTPUT"
	fi
}

pr_number="$(jq -r 'if type == "array" and length > 0 then (min_by(.number).number|tostring) else "" end' <<<"$pulls")"
if [[ -z "$pr_number" ]]; then
	emit "$sha landed on main with no pull request naming it, so no review and no required check ever applied to it"
	exit 1
fi

# THE LAST WORD BEFORE THE MERGE — not the newest run, and not the oldest.
#
# A re-run AFTER the merge answers about a tree that had already landed, so it
# must not clear the record of the merge it was absent for. That is why this was
# the oldest run, and that half is still true.
#
# But "oldest" only stands in for "before the merge" while there is one run
# before it. A red lane cleared by a re-run and then merged by a human has TWO,
# and the oldest is the run the merger looked at and correctly disregarded — so
# the alarm accused a merge whose required check was green at the moment it was
# made. Re-running is how a flaky lane is legitimately cleared, which made this
# fire on the ordinary repair rather than on anything.
#
# So: the latest verdict that had completed by merge time. Falling back to the
# oldest overall when none had, which is the fan-in's usual shape — `ci` posts
# minutes after the merge, and reading that run is the whole point of the alarm.
merged_at="$(jq -r 'if type == "array" and length > 0 then (min_by(.number).merged_at // "") else "" end' <<<"$pulls")"
verdict="$(jq -r --arg name "$required" --arg merged "$merged_at" '
	[.check_runs // [] | .[] | select(.name == $name)] as $named
	| ([$named[] | select($merged != "" and (.completed_at // "9999") <= $merged)]
	   | sort_by(.completed_at) | last)
	// ($named | sort_by(.completed_at // "9999") | .[0])
	// {}' <<<"$checks")"

if [[ "$(jq -r 'length' <<<"$verdict")" -eq 0 ]]; then
	echo "ok: $sha has no \`$required\` verdict to read (pull request #$pr_number) — an absent verdict is the shape of a decision that was taken, not a finding"
	exit 0
fi

# `success` passes; the three below are the OTHER ways to have no answer, and
# they are not adverse ones. A cancelled run is the case that would otherwise
# accuse: deleting the branch on merge cancels whatever was still running on it,
# so treating `cancelled` as a red verdict would report the tidy-up rather than
# the tree.
conclusion="$(jq -r '.conclusion // ""' <<<"$verdict")"
case "$conclusion" in
success)
	echo "ok: $sha merged behind a green \`$required\` (pull request #$pr_number)"
	;;
"" | cancelled | skipped | neutral)
	echo "ok: $sha has no \`$required\` verdict to read (pull request #$pr_number) — \`${conclusion:-still running}\` is an absent answer, not an adverse one"
	;;
*)
	emit "pull request #$pr_number landed, and its required \`$required\` check then reported \`$conclusion\` — the tree on main does not pass its own required check"
	exit 1
	;;
esac
