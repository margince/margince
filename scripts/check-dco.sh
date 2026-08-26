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
#
# AUTHOR-ONLY, held here rather than asked for. A certification nobody but the
# author may give is worth exactly what the check on it is worth: if any
# contributor can attest to anyone's commit, the trailer certifies that somebody
# typed a sha. Four conditions, each one a way the attestation could otherwise
# be somebody else's:
#
#   1. the attesting commit is itself signed off — an unsigned commit cannot
#      lend a signature it does not have;
#   2. its author is the author of the commit it attests to;
#   3. the identity it CLAIMS ("I, …") is that same author, so the sentence and
#      the commit cannot disagree about who is speaking;
#   4. it descends from that commit, which is the only order in which somebody
#      can attest to what they already wrote.
set -eu

base="${1:?usage: check-dco.sh <base-sha> <head-sha>}"
head="${2:?usage: check-dco.sh <base-sha> <head-sha>}"

range="${base}..${head}"
missing=0

signed_off() {
	git log -1 --format='%B' "$1" | grep -qiE '^Signed-off-by: .+ <.+@.+>'
}

# One record per accepted attestation: `<target-sha> <attesting-sha> <email>`.
# Collected across the WHOLE range before any commit is judged, because
# remediation is necessarily backward-looking.
attestations="$(mktemp)"
trap 'rm -f "$attestations"' EXIT HUP INT TERM

for sha in $(git rev-list --no-merges "$range"); do
	# Condition 1. An attestation carried by an unsigned commit is not read at
	# all, so a chain of unsigned commits cannot certify itself.
	signed_off "$sha" || continue
	author="$(git log -1 --format='%ae' "$sha" | tr '[:upper:]' '[:lower:]')"
	# The sha is matched as 40 hex characters, anchored. An abbreviation would
	# make the trailer's reach depend on how much of a sha somebody happened to
	# paste, and a prefix match would let one attestation silently cover a
	# second commit that shares its first characters.
	git log -1 --format='%B' "$sha" |
		sed -n -E 's/^DCO-Remediation-Commit: I, .+ <(.+@.+)>, hereby add my Signed-off-by to commit: ([0-9a-f]{40})[[:space:]]*$/\1 \2/p' |
		while read -r claimed target; do
			claimed="$(printf '%s' "$claimed" | tr '[:upper:]' '[:lower:]')"
			# Condition 3.
			[ "$claimed" = "$author" ] || continue
			echo "$target $sha $author" >>"$attestations"
		done
done

# attested_by <target-sha> — echoes the attesting sha, or nothing.
attested_by() {
	target="$1"
	target_author="$(git log -1 --format='%ae' "$target" | tr '[:upper:]' '[:lower:]')"
	grep "^${target} " "$attestations" 2>/dev/null | while read -r _ attester email; do
		# Condition 2.
		[ "$email" = "$target_author" ] || continue
		# Condition 4.
		git merge-base --is-ancestor "$target" "$attester" || continue
		echo "$attester"
		break
	done
}

for sha in $(git rev-list --no-merges "$range"); do
	if signed_off "$sha"; then
		continue
	fi
	attester="$(attested_by "$sha")"
	if [ -n "$attester" ]; then
		echo "DCO: commit ${sha} is unsigned but attested by its author in ${attester}"
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
	echo "its AUTHOR attests to it instead, in a later signed-off commit of theirs:" >&2
	echo "  DCO-Remediation-Commit: I, Name <mail@example.com>, hereby add my Signed-off-by to commit: <full-sha>" >&2
	echo "The attesting commit must be signed off and authored by the same person," >&2
	echo "under the same address the sentence names — nobody can attest for anybody else." >&2
	exit 1
fi

echo "DCO: all commits in ${range} are signed off"
