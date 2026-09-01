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

missing=0

# WHAT WAS WRONG WAS THE ASSERTION, NOT THE RANGE.
#
# `base..head` is every commit reachable from head and not from base, so when the
# base branch advances independently it still names exactly the commits this pull
# request introduces — the base's own new commits are excluded, which is the
# point. The range never needed changing.
#
# The ancestry check did. The caller hands over
# `github.event.pull_request.base.sha`, the tip of the base branch when the event
# fired, and a branch tip MOVES: anything merged to main after a pull request
# branched puts that tip off the head's ancestry, and asserting otherwise failed
# every open pull request on somebody else's merge.
#
# What survives is the question that assertion was reaching for — do these two
# share any history at all — asked in the form that cannot be answered by a busy
# afternoon.
#
# And it is `base`, not `git merge-base base head`. With criss-cross history
# there is more than one best common ancestor, merge-base names one of them, and
# a range starting there can reach a commit the OTHER ancestor already gave the
# base. This gate would then judge a commit the pull request did not introduce,
# and refuse it for somebody else's missing trailer.
if ! git merge-base "$base" "$head" >/dev/null 2>&1; then
	echo "DCO: ${base} and ${head} share no history, so there is no range of" >&2
	echo "commits this pull request introduces. Check the base and head it was given." >&2
	exit 1
fi
range="${base}..${head}"

# WHAT THE RANGE COVERS, asserted before it is trusted.
#
# `git rev-list A..B` does not complain when A is not behind B — it answers with
# whatever that range happens to mean, which for a base ahead of the head is
# nothing at all. The loops below then run zero times, `missing` stays 0, and
# this prints the same clean pass it prints for a fully signed branch. That is
# the one way a gate must not break: it reads a smaller history, reports PASS,
# and no assertion fails to say so.
#
# Dropping the ancestry assertion above does not retire that guard, it leaves it
# the whole job: a head already contained in the base yields an empty range, and
# the count below is the only thing that refuses it.
#
# Held HERE rather than at each call site, because the required caller is the
# one that had no guard: main-health.yml asserted its baseline's ancestry and
# ci.yml — the job the `ci` fan-in makes required — called this bare. A
# protection that lives in the advisory lane and not the blocking one protects
# nothing.

# Merges included: this is what the range HOLDS, not what needs a trailer. A
# range covering only merge commits is a real thing (a merge queue's own head),
# and reporting it as zero examined would be the same silent pass one step
# along, so the two counts are kept apart and both are printed.
present="$(git rev-list "$range" | wc -l | tr -d '[:space:]')"
if [ "$present" -eq 0 ]; then
	echo "DCO: the range ${range} holds no commits — this gate would report a clean" >&2
	echo "pass having read nothing. Check the base and head it was given." >&2
	exit 1
fi

signed_off() {
	git log -1 --format='%B' "$1" | grep -qiE '^Signed-off-by: .+ <.+@.+>'
}

# One record per accepted attestation: `<target-sha> <attesting-sha> <email>`.
# Collected across the WHOLE range before any commit is judged, because
# remediation is necessarily backward-looking.
attestations="$(mktemp)"
trap 'rm -f "$attestations"' EXIT HUP INT TERM

# Merges included, unlike the judging loop below: a merge commit needs no
# sign-off, but one that HAS a sign-off is a commit of its author's like any
# other, and CONTRIBUTING.md promises an attestation may live in any later
# signed-off commit of theirs.
for sha in $(git rev-list "$range"); do
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
			# Condition 3. The ADDRESS is the identity, and the name beside it
			# deliberately is not. A person signs commits under whatever display
			# name their account carries and writes their own name in prose, so
			# the two rarely match: both attestations on this repository's main
			# are authored by `luit-nfq` and `LarsGradion` while claiming
			# "Luitpold Alexander" and "Lars Jankowfsky". Comparing names would
			# reject exactly the attestations it exists to accept, and it buys
			# nothing — the address is what identifies the account that pushed.
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

judged=0
for sha in $(git rev-list --no-merges "$range"); do
	judged=$((judged + 1))
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

# The DENOMINATOR is the point of this line. "All commits are signed off" is
# what a gate that read nothing says too, and a number cannot be misread that
# way: a run reporting 0 of 0 says so where a run reporting 12 of 14 says
# something else.
echo "DCO: all ${judged} of ${present} commit(s) in ${range} are signed off ($((present - judged)) merge commit(s) need none)"
