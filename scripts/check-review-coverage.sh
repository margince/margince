#!/usr/bin/env bash
# check-review-coverage.sh — did a reviewer read the commit that fixes the review?
#
# A branch review reads the branch as it stood when the review was launched.
# Every commit made afterwards falls outside it — and the fixes for that
# review's own findings are always afterwards. So the normal, correct workflow
# produces an unreviewed commit by construction:
#
#   push  ->  review (commits 1..N)  ->  fix the findings (commit N+1)  ->  merge
#
# Commit N+1 carries changes a reviewer just judged risky enough to flag. It is
# the last commit anybody should want unread, and nothing on the pull request
# says it was not read. Four instances landed in one afternoon across three
# sessions; in the fourth, the author had just finished explaining the trap to
# somebody else and then walked into it on the next commit. Discipline does not
# fix a default.
#
# It is NOT a gate and will never be one. `ci` is the required check; this job
# is not, so a red here blocks nothing and states a fact. Prevention is a
# branch-protection decision and is not this script's to make.
#
# WHAT IT REPORTS:
#
#   - commits after the newest record a reviewer left. The range is named by
#     sha, so re-reviewing it is a copyable command rather than a guess.
#   - a record naming a commit this branch no longer has. A force-push after a
#     review destroys the evidence of what was read, and the reviewer's approval
#     goes on standing against a tree nobody compared it to.
#
# WHAT IT DELIBERATELY DOES NOT REPORT: a pull request nobody has reviewed yet.
# That is every pull request for most of its life, and an alarm that fires on
# the expected state is one that gets muted — the lesson check-merge-verdict.sh
# was written around after seventeen issues in five hours described a bypass
# working as intended.
#
# WHAT IT CANNOT SEE: a review that leaves no record on the pull request. An
# in-session review posts nothing here, so this speaks only for reviewers that
# do, and silence from it is not coverage. Said plainly rather than implied,
# because a coverage report read as exhaustive is worse than none.
#
# Reads its evidence from the environment rather than fetching it, so every arm
# is drivable from a fixture — the same shape check-merge-verdict.sh uses, and
# for the same reason: a reporter that can only be observed exiting 0 is not
# evidence of anything.
set -euo pipefail

head="${REVIEW_COVERAGE_HEAD:-}"
history="${REVIEW_COVERAGE_HISTORY:-}"
reviews="${REVIEW_COVERAGE_REVIEWS:-}"

if [[ -z "$head" ]]; then
	echo "FAIL: REVIEW_COVERAGE_HEAD is empty — there is no tip to compare against." >&2
	exit 2
fi
if [[ -z "$history" ]]; then
	echo "FAIL: REVIEW_COVERAGE_HISTORY is empty — without the branch's commits, every review would read as naming a commit the branch does not have." >&2
	exit 2
fi

# Empty is not the same as `[]`. A fetch that failed and a pull request nobody
# has reviewed both leave this unset if the caller is careless, and treating the
# first as the second would report full coverage on the day the API call breaks
# — which is the one direction this must not fail, since there is no red to
# notice.
if [[ -z "$reviews" ]]; then
	echo "FAIL: REVIEW_COVERAGE_REVIEWS is unset. Pass '[]' for a pull request with no reviews; unset cannot be told from a fetch that failed." >&2
	exit 2
fi

if ! jq -e 'type == "array"' >/dev/null 2>&1 <<<"$reviews"; then
	echo "FAIL: REVIEW_COVERAGE_REVIEWS is not a JSON array." >&2
	exit 2
fi

# The newest record per reviewer. A reviewer who read the branch twice is
# covered by their later reading, and reporting the earlier one would ask for a
# re-review that already happened.
latest="$(jq -c 'group_by(.reviewer) | map(sort_by(.at) | last) | sort_by(.reviewer)' <<<"$reviews")"

if [[ "$(jq 'length' <<<"$latest")" -eq 0 ]]; then
	echo "OK: no reviewer has left a record on this pull request yet — there is nothing to be behind."
	echo "    This says nothing about whether the branch was reviewed: a review that posts no record here is invisible to it."
	exit 0
fi

# ANCESTRY, not order. The history is `git rev-list --parents`, one commit per
# line followed by its parents, so what a review did not cover can be computed
# rather than guessed at from the listing order.
#
# Slicing the flat list was wrong the moment a pull request carried a merge:
# `rev-list` sorts by commit date, so a side branch written before the reviewed
# commit sorts below it and would have been read as already covered — silently,
# and on exactly the pull requests complicated enough to need this report.
#
# ONE awk pass, because the obvious shape is quadratic. Walking the graph in
# shell re-scans the history for every commit visited, and spawns a process to
# do it: on a long-lived pull request that is the report timing out rather than
# saying anything, which is worse than the wrong answer it replaced because
# nothing is left to read at all. awk indexes the edges once and walks them.
#
# commits_after <sha> is everything reachable from the tip that is NOT the
# reviewed commit or one of its ancestors — reachable(head) minus
# ancestors(reviewed), the set a re-review would have to read.
commits_after() {
	awk -v head="$head" -v reviewed="$1" '
		function walk(start,   stack, top, sha, kid, i) {
			delete reached
			stack[0] = start
			top = 1
			while (top > 0) {
				sha = stack[--top]
				if (sha in reached) continue
				reached[sha] = 1
				if (!(sha in parents)) continue
				split(parents[sha], kid, " ")
				for (i in kid) if (kid[i] != "") stack[top++] = kid[i]
			}
		}
		{
			order[NR] = $1
			rows = NR
			edges = ""
			for (i = 2; i <= NF; i++) edges = edges " " $i
			parents[$1] = edges
		}
		END {
			walk(reviewed)
			for (sha in reached) covered[sha] = 1
			walk(head)
			for (sha in reached) fromTip[sha] = 1
			# Oldest first, which is the order the commits were written: the
			# history arrives newest first, so it is read back to front. The
			# walk itself produces no useful order.
			for (i = rows; i >= 1; i--) {
				sha = order[i]
				if ((sha in fromTip) && !(sha in covered)) print sha
			}
		}
	' <<<"$history"
}

in_history() {
	awk -v want="$1" '$1 == want { found = 1; exit } END { exit !found }' <<<"$history"
}

findings=0
while read -r reviewer sha at; do
	if ! in_history "$sha"; then
		echo "STALE: ${reviewer} recorded a review of ${sha} at ${at}, and this branch no longer has that commit."
		echo "       A force-push after a review takes the evidence of what was read with it. The verdict still stands on the pull request against a tree nobody compared it to."
		findings=$((findings + 1))
		continue
	fi

	after="$(commits_after "$sha")"
	if [[ -z "$after" ]]; then
		echo "OK: ${reviewer} read ${sha}, which is the tip."
		continue
	fi

	count="$(grep -c . <<<"$after")"
	echo "BEHIND: ${count} commit(s) landed after ${reviewer} read ${sha}:"
	printf '%s\n' "$after"
	echo "       Ask ${reviewer} to read the range:"
	echo "         ${sha}..${head}"
	# NAMED, because the obvious remedy is not always the one that works. A
	# review records the commit id it was made against and a rebase rewrites
	# those ids, so a reviewer that tracks CONTENT has read everything while
	# this reads BEHIND — and it then declines an incremental re-review,
	# leaving a finding whose stated remedy does nothing. A refusal that names
	# an unreachable remedy is worse than one that names none.
	echo "       After a rebase, an incremental re-review may decline as already done;"
	echo "       \`@coderabbitai full review\` re-reads the whole changeset."
	findings=$((findings + 1))
done < <(jq -r '.[] | "\(.reviewer) \(.sha) \(.at)"' <<<"$latest")

if [[ "$findings" -eq 0 ]]; then
	echo "OK: every review on this pull request was made against the tip."
	exit 0
fi

echo
echo "This blocks nothing — \`ci\` is the required check and this is not it. It reports that code a reviewer flagged as worth changing was changed afterwards and not read again."
exit 1
