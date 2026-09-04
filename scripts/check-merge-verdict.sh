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
#   - the check never reported. It was still running, or never started, when the
#     merge landed — #2504's own case, and the one a "was it green?" question
#     cannot see, because an absent verdict is not a red one.
#   - the check reported and was not green.
#   - the check went green AFTER the merge. The merge was decided on an earlier,
#     unfinished state; that the answer turned out well is luck, not a gate.
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

merged_at="$(jq -r --arg n "$pr_number" '[.[] | select((.number|tostring) == $n)] | .[0].merged_at // ""' <<<"$pulls")"

# The oldest matching run, not the newest. A re-run after the merge answers
# about a tree that had already landed, and taking the latest verdict would let
# one clear the record of the merge it was not present for.
verdict="$(jq -r --arg name "$required" '
	[.check_runs // [] | .[] | select(.name == $name)]
	| sort_by(.completed_at // "9999") | .[0] // {}' <<<"$checks")"

if [[ "$(jq -r 'length' <<<"$verdict")" -eq 0 ]]; then
	emit "pull request #$pr_number merged before its required \`$required\` check reported at all — an absent verdict, which is not the same as a green one"
	exit 1
fi

conclusion="$(jq -r '.conclusion // ""' <<<"$verdict")"
if [[ "$conclusion" != "success" ]]; then
	emit "pull request #$pr_number merged while its required \`$required\` check was \`${conclusion:-still running}\`"
	exit 1
fi

completed_at="$(jq -r '.completed_at // ""' <<<"$verdict")"
# Compared as STRINGS, which is right for these two and only these two: the API
# writes both as ISO-8601 in UTC with the same fixed width, so lexical order is
# chronological order. It would not be for a mixed-offset timestamp, and neither
# field is ever one.
if [[ -n "$merged_at" && -n "$completed_at" && "$completed_at" > "$merged_at" ]]; then
	emit "pull request #$pr_number merged at $merged_at, before its required \`$required\` check finished at $completed_at — the merge was decided on an unfinished run"
	exit 1
fi

echo "ok: $sha merged behind a green \`$required\` (pull request #$pr_number)"
