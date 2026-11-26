#!/usr/bin/env bash
# ci-integration-proven.sh — on the merge queue, did an earlier run of the
# entry's pull request already pass the integration lane on this exact tree?
#
# A group of one entry on an unchanged main has the tree the pull request's last
# run merged and tested. Running the eight shards again proves nothing new and
# holds a dozen runners the other queue entries wait for.
#
# The proof is a `proven-integration-<tree>` artifact, which the lane's fan-in
# uploads once its shards pass, from a run whose head is the entry's pull
# request head. That anchor is what makes the artifact trustworthy: such a run
# executed the pull request's own workflow, which is the code this merge lands.
# A run of any other head could name any tree. Every `integration / ` job of that
# run must also have succeeded, since the fan-in does not wait for all of them.
#
# Prints `proven=true` or `proven=false` for $GITHUB_OUTPUT. A failed API read
# prints `proven=false` with a warning: the answer is then to run the lane.
#
# Reads REPO, MERGE_GROUP_SHA and MERGE_GROUP_REF
# (refs/heads/gh-readonly-queue/<base>/pr-<n>-<base sha>).
set -euo pipefail

repo="${REPO:?REPO is empty: pass github.repository}"
group_sha="${MERGE_GROUP_SHA:?MERGE_GROUP_SHA is empty: pass github.event.merge_group.head_sha}"
group_ref="${MERGE_GROUP_REF:?MERGE_GROUP_REF is empty: pass github.event.merge_group.head_ref}"

not_proven() {
	echo "$1" >&2
	echo "proven=false"
	exit 0
}

read_failed() {
	echo "::warning::$1 The integration lane runs." >&2
	echo "proven=false"
	exit 0
}

if [[ ! "$group_ref" =~ /pr-([0-9]+)-[0-9a-f]+$ ]]; then
	read_failed "No pull request number in the merge group ref '$group_ref'."
fi
pr="${BASH_REMATCH[1]}"

tree="$(gh api "repos/$repo/git/commits/$group_sha" --jq .tree.sha)" ||
	read_failed "Could not read the tree of $group_sha."
pr_head="$(gh api "repos/$repo/pulls/$pr" --jq .head.sha)" ||
	read_failed "Could not read the head of pull request #$pr."

runs="$(gh api "repos/$repo/actions/artifacts?name=proven-integration-$tree&per_page=100" \
	--jq ".artifacts[] | select(.expired | not) | select(.workflow_run.head_sha == \"$pr_head\") | .workflow_run.id")" ||
	read_failed "Could not list the proven-integration artifacts."

for run in $(printf '%s\n' "$runs" | sort -u); do
	path="$(gh api "repos/$repo/actions/runs/$run" --jq .path)" ||
		read_failed "Could not read run $run."
	[[ "$path" == ".github/workflows/ci.yml" ]] || continue
	# Counts the lane's jobs and those that did not succeed, in one read.
	verdict="$(gh api --paginate "repos/$repo/actions/runs/$run/jobs?per_page=100" \
		--jq '.jobs[] | select(.name | startswith("integration / ")) | .conclusion // "pending"' |
		awk '{ n++ } $0 != "success" { bad++ } END { printf "%d %d", n, bad }')" ||
		read_failed "Could not read the jobs of run $run."
	read -r jobs bad <<<"$verdict"
	if [[ "$jobs" -gt 0 && "$bad" -eq 0 ]]; then
		echo "Run $run of pull request #$pr ($pr_head) passed all $jobs integration jobs on tree $tree." >&2
		echo "proven=true"
		exit 0
	fi
	echo "Run $run names tree $tree, but $bad of its $jobs integration jobs did not succeed." >&2
done

not_proven "No run of pull request #$pr ($pr_head) passed the integration lane on tree $tree."
