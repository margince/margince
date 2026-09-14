#!/usr/bin/env bash
# The commit a release patch is cut FROM.
#
# A patch is applied by somebody whose tree is the last release they RECEIVED.
# The base therefore has to be what was last published, and it used to be
# `github.event.before` — the ref's previous tip, which is a fact about this
# push and not about anybody's installation. The two agree only while every
# push publishes.
#
# They diverge whenever a lane does not. A cancelled run (the release group
# holds one queued slot, so a merge evicts the run behind it) and a failed one
# both leave a commit published by nobody, and the NEXT patch then starts after
# it — so that commit's files are missing from every patch a consumer will ever
# apply, and nothing says so. Two instances in one day is what made this a
# defect with a history rather than a hazard.
#
# So the base is the last published revision, recorded in this repository by the
# tag the publish job moves AFTER a publish succeeds. Ordering is what makes the
# tag safe as a cache of the dist service's own answer: a tag that failed to
# move leaves the next patch WIDER than necessary, which a consumer applies
# without harm, while a tag that moved too early is the narrow patch this exists
# to prevent.
#
# Usage: scripts/release-patch-base.sh <published-ref> <before> <fallback-ref>
#   published-ref  the tag naming the last published revision ("released")
#   before         github.event.before, empty on a manual dispatch
#   fallback-ref   what to diff from when neither is available (HEAD~1)
#
# It prints the base commit, or nothing at all — and nothing is a real answer:
# the first release of a repository has no previous published state, and a
# release drafted without a patch is the honest shape for it.
set -euo pipefail

published="${1-}"
before="${2-}"
fallback="${3-}"

# resolvable answers whether a name is a commit this checkout can reach. A
# force-push discards its old tip, so a `before` the fetched history no longer
# carries is not an error to report — it is simply not a base.
resolvable() {
	[ -n "$1" ] && git rev-parse --verify --quiet "$1^{commit}" >/dev/null 2>&1
}

# An all-zeros `before` is branch creation: there is no previous state at all.
zeros() {
	printf '%s' "$1" | grep -qE '^0+$'
}

# THE PUBLISHED REVISION WINS, and it wins even when it is not an ancestor of
# the commit being released. A base that no longer sits on this branch still
# names the tree a consumer is holding, and the diff from it is exactly the
# increment they need — which is the same reasoning the previous-tip base
# already carried, applied to a better answer about whose tree it is.
if resolvable "$published"; then
	git rev-parse --verify "$published^{commit}"
	exit 0
fi

# No published revision recorded: either the first release, or a repository
# whose tag has not been written yet. The previous tip is the old behaviour and
# the best remaining guess — it is right whenever the lane before this one did
# publish, which is the ordinary case.
if [ -n "$before" ] && ! zeros "$before" && resolvable "$before"; then
	git rev-parse --verify "$before^{commit}"
	exit 0
fi

if resolvable "$fallback"; then
	git rev-parse --verify "$fallback^{commit}"
	exit 0
fi

# Nothing to diff from. Printing nothing is the interface: the caller drafts a
# release with no patch, which is correct for a first release and honest for a
# history this checkout cannot reach.
exit 0
