#!/usr/bin/env bash
# Turn the squash-merge subjects since the last release into release notes.
#
# Subjects follow conventional commits: `feat(scope): …`, `fix(scope): …`. A
# `!` before the colon is breaking. Only what a user or operator can notice is
# listed: breaking, feat, fix and perf. refactor, test, docs, ci, chore and the
# dependency updates without a prefix are left to the compare view.
#
# A release body holds at most 125,000 characters, and two weeks of merges here
# list about 400 subjects. The caller passes the room its own text leaves. Over
# it, fixes and speed-ups, then features, collapse into a count. Breaking
# changes are never cut, since an operator reads them before an upgrade, so
# breaking changes too long for the room refuse.
#
# WHAT IT READS: subjects on stdin and its arguments. No git, no network, so its
# own test can feed subjects and judge the answer.
#
# Usage: git log --first-parent --format=%s v1.0.0..v1.1.0 |
#          bash scripts/release-notes.sh v1.0.0 v1.1.0 margince/margince [budget]
set -euo pipefail

previous="${1:-}"
tag="${2:-}"
repo="${3:-}"
budget="${4:-100000}"

if [[ -z "$previous" || -z "$tag" || -z "$repo" ]]; then
	echo "release-notes: pass the previous tag, this tag and owner/repo; subjects on stdin." >&2
	exit 1
fi

breaking="" added="" fixed="" faster=""
n_added=0 n_fixed=0 n_faster=0
subject_re='^([a-z]+)(\(([^)]*)\))?(!)?: (.+)$'
while IFS= read -r subject || [[ -n "$subject" ]]; do
	[[ "$subject" =~ $subject_re ]] || continue
	type="${BASH_REMATCH[1]}" scope="${BASH_REMATCH[3]}" bang="${BASH_REMATCH[4]}" text="${BASH_REMATCH[5]}"
	line="- ${scope:+**$scope:** }$text"
	if [[ -n "$bang" ]]; then
		breaking+="$line"$'\n'
		continue
	fi
	case "$type" in
		feat) added+="$line"$'\n'; n_added=$((n_added + 1)) ;;
		fix) fixed+="$line"$'\n'; n_fixed=$((n_fixed + 1)) ;;
		perf) faster+="$line"$'\n'; n_faster=$((n_faster + 1)) ;;
	esac
done

compare="https://github.com/$repo/compare/$previous...$tag"

# section <heading> <lines> prints a section, or nothing for an empty list.
section() {
	[[ -n "$2" ]] && printf '### %s\n\n%s\n' "$1" "$2"
	return 0
}

render() {
	section Breaking "$breaking"
	section Added "$added"
	section Fixed "$fixed"
	section Faster "$faster"
	printf 'Every change since %s: %s\n' "$previous" "$compare"
}

# collapsed <count> prints the line a shortened section keeps, or nothing.
collapsed() {
	(($1 > 0)) && printf -- '- %d more, listed in the compare view.\n' "$1"
	return 0
}

notes="$(render)"
if ((${#notes} > budget)); then
	fixed="$(collapsed "$n_fixed")" faster="$(collapsed "$n_faster")"
	fixed="${fixed:+$fixed$'\n'}" faster="${faster:+$faster$'\n'}"
	notes="$(render)"
fi
if ((${#notes} > budget)); then
	added="$(collapsed "$n_added")"
	added="${added:+$added$'\n'}"
	notes="$(render)"
fi
if ((${#notes} > budget)); then
	echo "release-notes: the shortest notes, every section but Breaking counted, need ${#notes} characters; the room is $budget." >&2
	echo "  Shorten the draft's own text, or the breaking pull request titles." >&2
	exit 1
fi
printf '%s\n' "$notes"
