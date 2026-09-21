# Counts a Go source's CODE and COMMENT lines, for the two comment gates.
#
# Spliced after scripts/lib-commentscan.awk, which owns the one reading of where
# a comment starts: `awk -f lib-commentscan.awk -f lib-commentcount.awk`. A third
# hand-written `^//` test is what these gates shipped with, and it counted every
# line of a `/* … */` block as code — so a change could buy budget by writing its
# prose in block comments, and the tree's own ratio read low for the same reason.
#
# MODE=diff reads a unified diff and judges ADDED and REMOVED lines, taking each
# file from the `+++` header. MODE=tree reads whole files named as arguments.
#
# Block state is per file, reset on the header in diff mode and on FNR==1 in
# tree mode. A diff is fragments rather than a whole file, so a block a hunk
# opens and never closes stops at the file boundary instead of blinding the rest
# of the run.
#
# Prints "<code> <comment> <removed-comment>".

function classify(line,   c) {
	if (line ~ /^[ \t]*$/) return "blank"
	if (line ~ /^[ \t]*\/\/ SPDX-/) return "blank"
	c = codeOf(line)
	gsub(/^[ \t]+|[ \t]+$/, "", c)
	return (c == "") ? "comment" : "code"
}

function resetBlock() { INBLOCK = 0; STACK = ""; STACKIN = "" }

BEGIN { if (MODE == "") MODE = "tree" }

MODE == "tree" && FNR == 1 { resetBlock() }

MODE == "tree" {
	kind = classify($0)
	if (kind == "code") k++
	else if (kind == "comment") c++
	next
}

MODE == "diff" && /^--- / { next }

MODE == "diff" && /^\+\+\+ / {
	path = substr($0, 7)
	want = (path ~ /\.go$/) && (path !~ /_gen\.go$/) &&
		(path !~ /\.gen\.go$/) && (path !~ /(^|\/)doc\.go$/)
	resetBlock()
	next
}

MODE == "diff" && /^@@/ { resetBlock(); next }

MODE == "diff" && !want { next }

MODE == "diff" && /^\+/ {
	kind = classify(substr($0, 2))
	if (kind == "code") k++
	else if (kind == "comment") c++
	next
}

# A removed line is judged on its own, so the block state the added lines are
# tracking is not disturbed by it.
MODE == "diff" && /^-/ {
	keep = INBLOCK; keepStack = STACK; keepIn = STACKIN
	kind = classify(substr($0, 2))
	INBLOCK = keep; STACK = keepStack; STACKIN = keepIn
	if (kind == "comment") gone++
	next
}

END { printf "%d %d %d\n", k + 0, c + 0, gone + 0 }
