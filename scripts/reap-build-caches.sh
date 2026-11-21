#!/usr/bin/env bash
# reap-build-caches.sh — delete the build caches that nothing can restore.
#
# `.github/actions/go-build-cache` keys every entry
# `go-build-<flavor>-<os>-<arch>-<deps-hash>-<commit-sha>` and falls back through
# `restore-keys` to the newest entry sharing the deps hash.
# `.github/actions/frontend-cache-key` keys `frontend-incremental-…` the same way. A lookup searches the
# reader's own ref, then its base branch, then the default branch, so `main` and
# `develop` each serve readers of their own. Within one ref and one deps hash only
# the NEWEST entry is ever read: every older one is unreachable by construction,
# and the only thing it can still do is take up room.
#
# Room is the problem. A repository gets 10 GB of Actions cache, and one push to
# main writes about 5 GB of Go build cache across the two flavours. Two
# generations fill the quota on their own, and GitHub then evicts by
# least-recently-used — which reaches the small entries first, because
# node-cache, gate-binaries and setup-go are read once per run each while a 3 GB
# build cache is read by every Go job. The cheap caches are evicted by the
# expensive ones, and the lanes that depended on them quietly get slower.
#
# So this deletes the superseded generations rather than waiting for eviction to
# choose worse victims. It is deliberately narrow: only those two key kinds, only
# entries that are not the newest of their ref and deps hash. Everything else — including
# an older deps hash, which restore-keys CAN still fall back to after a go.sum
# bump — is left alone.
#
# DRY_RUN=1 prints the plan and deletes nothing. REF=refs/heads/<branch> limits
# the sweep to that ref, which is how cache-warm.yml prunes after it saves.
set -euo pipefail

repo="${REPO:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}"
dry_run="${DRY_RUN:-0}"
only_ref="${REF:-}"

# Creation time first, then sorted here rather than by the API. The ordering IS
# the correctness argument — "keep the first of each group" only means "keep the
# newest" while the list is newest-first — and this deletes things, so it does
# not rest on a query parameter a server is free to ignore. RFC 3339 timestamps
# sort lexicographically, so a plain reverse sort is the right one.
caches="$(gh api "repos/$repo/actions/caches?per_page=100" \
	--paginate --jq '.actions_caches[] | [.created_at, .id, .ref, .key, .size_in_bytes] | @tsv' |
	sort -r)"

# An empty listing is ambiguous — a repository with no caches looks exactly like
# a token that cannot read them, and the second one must not pass for a clean
# sweep. There is always at least one cache on a repository whose CI has run.
if [[ -z "$caches" ]]; then
	echo "reap-build-caches: $repo listed no caches at all — refusing to report a clean sweep over a read that returned nothing" >&2
	exit 1
fi

seen_groups=""
deleted=0
freed=0
kept=0
kept_go_build=0

while IFS=$'\t' read -r created id ref key size; do
	[[ -n "$id" ]] || continue
	case "$key" in
	go-build-* | frontend-incremental-*) ;;
	*) continue ;;
	esac

	# The group is the ref plus the key without its trailing commit sha. Grouped
	# across refs, a newer develop entry would delete the main entry that pull
	# requests into main and the merge queue restore.
	if ! [[ "$ref" =~ ^refs/[^[:space:]]+$ ]]; then
		echo "reap-build-caches: cache $key has no ref ('$ref'); without one this script would group two branches together" >&2
		exit 1
	fi
	[[ -z "$only_ref" || "$ref" == "$only_ref" ]] || continue
	group="$ref|${key%-*}"
	# Checked as a commit sha, not merely as "something after a dash": every
	# segment of the key is dash-separated, so a truncated key still splits
	# cleanly and would silently group two flavours together — and grouping is
	# what decides which entry gets deleted.
	if ! [[ "${key##*-}" =~ ^[0-9a-f]{40}$ ]]; then
		echo "reap-build-caches: cache key $key does not end in a commit sha; the key shape changed and this script would group it wrongly" >&2
		exit 1
	fi

	case " $seen_groups " in
	*" $group "*)
		printf 'delete  %6s MB  %s  %s  (superseded)\n' "$((size / 1048576))" "$ref" "$key"
		if [[ "$dry_run" != "1" ]]; then
			gh api -X DELETE "repos/$repo/actions/caches/$id" --silent
		fi
		deleted=$((deleted + 1))
		freed=$((freed + size))
		;;
	*)
		printf 'keep    %6s MB  %s  %s  (newest, %s)\n' "$((size / 1048576))" "$ref" "$key" "$created"
		seen_groups="$seen_groups $group"
		kept=$((kept + 1))
		[[ "$key" != go-build-* ]] || kept_go_build=$((kept_go_build + 1))
		;;
	esac
done <<<"$caches"

if [[ "$kept_go_build" -eq 0 ]]; then
	echo "reap-build-caches: no go-build-* cache found${only_ref:+ on $only_ref} — the key prefix changed, so this ran over nothing" >&2
	exit 1
fi

verb="freed"
[[ "$dry_run" = "1" ]] && verb="would free"
echo "reap-build-caches: kept $kept, deleted $deleted, $verb $((freed / 1048576)) MB"
