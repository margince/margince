#!/usr/bin/env bash
# Keep a Changelog gate: one section per change type per release.
#
# CHANGELOG.md declares it follows Keep a Changelog, which expects one
# `### Changed` under a release, not three. Three shipped anyway, and the way
# they arrived is the reason this exists: each author appended a heading rather
# than scrolling for the existing one, and nothing in the file gives a signal
# that two others already sit above and below. A reader looking for what
# changed has to find all of them, and a markdown linter flags the duplicates
# on every unrelated pull request that touches the file.
#
# WHAT IT READS. The headings, and nothing else. The change types are taken
# FROM the file rather than from a list maintained here: any `### ` heading
# under a `## [` release is a section, so a release that grows a `### Removed`
# is held the same day it appears, with nothing to remember to add.
#
# WHAT IT IS NOT. It does not judge whether an entry sits under the right
# heading, whether the wording is good, or whether the file is complete. It
# says only that a reader looking for one change type finds one place to look.
set -euo pipefail

changelog="${1:-CHANGELOG.md}"
if [[ ! -f "$changelog" ]]; then
	echo "check-changelog-sections: $changelog does not exist" >&2
	exit 1
fi

# The awk below reports, so a release with no duplicate prints nothing and the
# exit status carries the verdict. `releases` is counted so that a rewrite
# which renamed the release heading — leaving nothing to walk — fails loudly
# rather than passing as clean: a census that finds nothing certifies nothing.
verdict=$(awk '
	/^## \[/ { release = $0; releases++; delete seen; next }
	release != "" && /^### / {
		sections++
		if ($0 in seen) {
			printf "%s: %s appears %d times under %s\n", FILENAME, $0, ++count[$0] + 1, release
			bad = 1
		}
		seen[$0] = 1
	}
	END {
		if (releases == 0) {
			print "no `## [release]` heading found — this gate read nothing and cannot certify the file"
			bad = 1
		}
		if (releases > 0 && sections == 0) {
			print "no `### ` section found under any release — this gate read nothing and cannot certify the file"
			bad = 1
		}
		exit bad
	}
' "$changelog") && status=0 || status=$?

if [[ $status -ne 0 ]]; then
	echo "$verdict" >&2
	echo "FAIL: a change type is split across more than one section — merge them, so a reader finds one list per type" >&2
	exit 1
fi

echo "OK: changelog-sections — one section per change type in every release"
