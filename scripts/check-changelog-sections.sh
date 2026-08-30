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
# exit status carries the verdict.
#
# WHAT IT READS, AND WHAT IT STEPS OVER. Headings outside fenced code, and
# nothing else. An entry that quotes a changelog — this file's own entries
# do — carries `## [1.0.0]` and `### Changed` inside a fence, and a scanner that
# counted those would refuse a correct file for describing itself.
#
# EVERY release is judged on its own. Both the duplicate check and the
# read-nothing floor were global once, which meant one release with a section
# excused a release with none, and a diagnostic count carried over from the
# release above. A `##` heading that is NOT a release ends the release it
# follows rather than lending its own `###` children to it.
verdict=$(awk '
	function leaveRelease() {
		if (release != "" && sections == 0) {
			printf "%s: %s carries no `### ` section — this gate read nothing under it and cannot certify it\n",
				FILENAME, release
			bad = 1
		}
		release = ""; sections = 0
		delete seen; delete count
	}
	# The delimiter that OPENED the fence is what closes it. A ``` line inside a
	# ~~~ block is content, and a scanner that closed on it would go on to read
	# the quoted headings below as real ones — refusing a correct file.
	# Up to THREE leading spaces. Four is an indented code block, not a fence,
	# so a four-space line that looked like one could open a block the file
	# never closes and swallow every heading below it.
	/^ {0,3}(```|~~~)/ {
		run = $0
		sub(/^ */, "", run)
		marker = substr(run, 1, 1)
		width = 0
		while (substr(run, width + 1, 1) == marker) width++
		trailing = substr(run, width + 1)
		if (fence == "") {
			# An opening fence may carry an info string, and often does:
			# ```bash. Only the run itself is remembered.
			fenceChar = marker
			fenceWidth = width
			fence = "open"
		} else if (marker == fenceChar && width >= fenceWidth && trailing ~ /^[ \t]*$/) {
			# A CLOSING fence carries nothing after it, so ```bash inside a
			# backtick block is content and leaves the fence open.
			fence = ""
		}
		next
	}
	fence != "" { next }
	/^## / {
		leaveRelease()
		if ($0 ~ /^## \[/) { release = $0; releases++ }
		next
	}
	release != "" && /^### / {
		heading = $0
		sub(/[ \t]+$/, "", heading)
		sections++
		if (heading in seen) {
			printf "%s: %s appears %d times under %s\n", FILENAME, heading, ++count[heading] + 1, release
			bad = 1
		}
		seen[heading] = 1
	}
	END {
		leaveRelease()
		if (releases == 0) {
			printf "%s: no `## [release]` heading found — this gate read nothing and cannot certify the file\n", FILENAME
			bad = 1
		}
		exit bad
	}
' "$changelog") && status=0 || status=$?

if [[ $status -ne 0 ]]; then
	echo "$verdict" >&2
	# One sentence for every refusal class. A duplicate-specific line printed
	# over a read-nothing refusal sends a maintainer looking for sections to
	# merge that were never split.
	echo "FAIL: $changelog does not present one section per change type per release — see the line(s) above" >&2
	exit 1
fi

echo "OK: changelog-sections — one section per change type in every release"
