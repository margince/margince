#!/usr/bin/env bash
# ci-integration-proven.sh — did an earlier run already pass the integration
# lane on this exact tree? Asked on the merge queue, and on a pull request run's
# second or later attempt ("Re-run all jobs").
#
# On a re-run the proof is an earlier attempt of the same run: it has the same
# head and the same workflow, so every integration job of that attempt having
# succeeded is enough. Each attempt is read on its own, since the run's job list
# shows only the newest attempt, where the lane has not run yet. Sonar merges
# that attempt's shard coverage, which expires after a day, so a re-run that
# finds any of it gone runs the lane again.
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
# Reads REPO, and MERGE_GROUP_SHA and MERGE_GROUP_REF
# (refs/heads/gh-readonly-queue/<base>/pr-<n>-<base sha>) on the merge queue, or
# RUN_ID and RUN_ATTEMPT on a pull request re-run.
set -euo pipefail

repo="${REPO:?REPO is empty: pass github.repository}"

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

# lane_verdict JOBS_ENDPOINT prints "<lane jobs> <lane jobs that did not succeed> <shard jobs>".
lane_verdict() {
	gh api --paginate "$1" \
		--jq '.jobs[] | select(.name | startswith("integration / ")) | "\(.conclusion // "pending") \(.name)"' |
		awk '{ n++ } $1 != "success" { bad++ } /^[a-z_]+ integration \/ integration shard / { shards++ }
			END { printf "%d %d %d", n, bad, shards }'
}

# kept_shard_pods ATTEMPT prints how many of that attempt's shard coverage
# artifacts have not expired.
kept_shard_pods() {
	gh api --paginate "repos/$repo/actions/runs/$run_id/artifacts?per_page=100" \
		--jq "[.artifacts[] | select(.expired | not) | select(.name | test(\"^integration-shard-[0-9]+-attempt-$1\$\"))] | length" |
		awk '{ n += $1 } END { print n + 0 }'
}

if [[ -z "${MERGE_GROUP_SHA:-}" ]]; then
	run_id="${RUN_ID:?RUN_ID is empty: pass github.run_id}"
	attempt="${RUN_ATTEMPT:?RUN_ATTEMPT is empty: pass github.run_attempt}"
	for ((earlier = attempt - 1; earlier >= 1; earlier--)); do
		verdict="$(lane_verdict "repos/$repo/actions/runs/$run_id/attempts/$earlier/jobs?per_page=100")" ||
			read_failed "Could not read the jobs of attempt $earlier of run $run_id."
		read -r jobs bad shards <<<"$verdict"
		[[ "$jobs" -gt 0 && "$bad" -eq 0 ]] || continue
		kept="$(kept_shard_pods "$earlier")" ||
			read_failed "Could not list the artifacts of run $run_id."
		if [[ "$kept" -lt "$shards" ]]; then
			not_proven "Attempt $earlier of run $run_id passed the lane, but $((shards - kept)) of its $shards shards' coverage has expired."
		fi
		echo "Attempt $earlier of run $run_id passed all $jobs integration jobs on this tree." >&2
		echo "proven=true"
		exit 0
	done
	not_proven "No earlier attempt of run $run_id passed the integration lane."
fi

group_sha="$MERGE_GROUP_SHA"
group_ref="${MERGE_GROUP_REF:?MERGE_GROUP_REF is empty: pass github.event.merge_group.head_ref}"

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
	verdict="$(lane_verdict "repos/$repo/actions/runs/$run/jobs?per_page=100")" ||
		read_failed "Could not read the jobs of run $run."
	read -r jobs bad _ <<<"$verdict"
	if [[ "$jobs" -gt 0 && "$bad" -eq 0 ]]; then
		echo "Run $run of pull request #$pr ($pr_head) passed all $jobs integration jobs on tree $tree." >&2
		echo "proven=true"
		exit 0
	fi
	echo "Run $run names tree $tree, but $bad of its $jobs integration jobs did not succeed." >&2
done

not_proven "No run of pull request #$pr ($pr_head) passed the integration lane on tree $tree."
