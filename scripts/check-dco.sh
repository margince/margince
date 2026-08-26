#!/bin/sh
# Developer Certificate of Origin gate: every commit introduced by a PR must
# carry a `Signed-off-by:` trailer (git commit -s). Called by the `dco` CI job
# with the PR's base and head SHAs; the range excludes history already on the
# base branch, so only the PR's own commits are checked.
#
# A commit already on a protected branch cannot grow a trailer — the history is
# immutable and rewriting it is not on the table — so an unsigned one is
# attested instead, by its own author, in a later commit:
#
#   DCO-Remediation-Commit: I, A Name <a@example.com>, hereby add my
#   Signed-off-by to commit: <full 40-character sha>
#
# (one line, wrapped here only for the reader). That is the ecosystem's own
# form, and it is what lets a hole CLOSE. Without it the only answer to an
# unsigned commit is to move the baseline past it, which buys a green gate by
# reading less history every time — the failure mode this repository names
# outright: a census that can fail short has already failed.
set -eu

base="${1:?usage: check-dco.sh <base-sha> <head-sha>}"
head="${2:?usage: check-dco.sh <base-sha> <head-sha>}"

range="${base}..${head}"
missing=0

# Attestations are collected from the WHOLE range before any commit is judged,
# because remediation is necessarily backward-looking: the commit that attests
# is always later than the one it attests for.
#
# The sha is matched as 40 hex characters, anchored. An abbreviation would make
# the trailer's reach depend on how much of a sha somebody happened to paste,
# and a prefix match would let one attestation silently cover a second commit
# that shares its first characters.
remediated="$(
	git log --format='%B' "$range" |
		sed -n -E 's/^DCO-Remediation-Commit: I, .+ <.+@.+>, hereby add my Signed-off-by to commit: ([0-9a-f]{40})[[:space:]]*$/\1/p'
)"

for sha in $(git rev-list --no-merges "$range"); do
	if git log -1 --format='%B' "$sha" | grep -qiE '^Signed-off-by: .+ <.+@.+>'; then
		continue
	fi
	# `grep -x` on the collected list: an attestation names one whole sha, and
	# the commit it names is the only one it excuses.
	if printf '%s\n' "$remediated" | grep -qxF "$sha"; then
		echo "DCO: commit ${sha} is unsigned but attested by its author in a later commit"
		continue
	fi
	subject="$(git log -1 --format='%s' "$sha")"
	echo "DCO: commit ${sha} missing Signed-off-by trailer: ${subject}" >&2
	missing=1
done

if [ "$missing" -ne 0 ]; then
	echo "" >&2
	echo "Every commit must be signed off (git commit -s). See CONTRIBUTING.md." >&2
	echo "A commit already on a protected branch cannot be signed after the fact;" >&2
	echo "its AUTHOR attests to it instead, in a later signed-off commit message:" >&2
	echo "  DCO-Remediation-Commit: I, Name <mail@example.com>, hereby add my Signed-off-by to commit: <full-sha>" >&2
	exit 1
fi

echo "DCO: all commits in ${range} are signed off"
