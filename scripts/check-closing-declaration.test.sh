#!/usr/bin/env bash
# The declaration check's own test — what it accepts, and what it must not.
#
# The second half carries the finding. This check exists because a pull request
# body said "Closes the residual half of #548" and closed nothing, so a version
# of it that read the PROSE would pass that exact pull request and report the
# defect as absent — a green check over the one case it was written for. That
# case is here by name.
#
# Usage: bash scripts/check-closing-declaration.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK="$SCRIPT_DIR/check-closing-declaration.sh"

FAILURES=0

# declared <refs> <body> <reason>
declared() {
	local refs="$1" body="$2" reason="$3" out
	if ! out="$(CLOSING_DECL_REFS="$refs" CLOSING_DECL_BODY="$body" bash "$CHECK" 2>&1)"; then
		printf 'FAIL: asked for a declaration when %s\n  refs=%q body=%q\n  %s\n' "$reason" "$refs" "$body" "$out"
		FAILURES=$((FAILURES + 1))
	fi
}

# undeclared <refs> <body> <reason>
undeclared() {
	local refs="$1" body="$2" reason="$3" out
	if out="$(CLOSING_DECL_REFS="$refs" CLOSING_DECL_BODY="$body" bash "$CHECK" 2>&1)"; then
		printf 'FAIL: accepted a pull request that %s\n  refs=%q body=%q\n' "$reason" "$refs" "$body"
		FAILURES=$((FAILURES + 1))
		return
	fi
	# The message has to carry the fix. A warning nobody can act on is noise,
	# and this one fires on a habit rather than on a fault.
	if [[ "$out" != *"Closes: none"* || "$out" != *"Closes #"* ]]; then
		printf 'FAIL: the warning does not show both declarations:\n%s\n' "$out"
		FAILURES=$((FAILURES + 1))
	fi
}

declared "1234" "" "GitHub records a closing reference"
declared "1234
5678" "" "a pull request closing two issues is still declaring"
declared "" "Closes: none" "the author said it closes nothing"
declared "" "Some prose.

closes: NONE

More prose." "the declaration is a line of its own, whatever its case"

# THE WORKED EXAMPLE. GitHub parses a closing keyword only when the number
# follows it directly, so this body closed nothing while reading as if it did.
undeclared "" "Closes the residual half of #548" \
	"names an issue in prose GitHub does not parse, which is the whole finding"
undeclared "" "" "says nothing at all"
undeclared "" "Refs #548" "references an issue without closing it and without saying so"
# Mentioned in passing is not a declaration: the token has to be the line.
undeclared "" "I considered writing Closes: none here but this one does close something." \
	"only mentions the token inside a sentence"

if [[ "$FAILURES" -gt 0 ]]; then
	echo "check-closing-declaration: $FAILURES failure(s)" >&2
	exit 1
fi
echo "OK: check-closing-declaration — 8 case(s), including the prose that closed nothing"
