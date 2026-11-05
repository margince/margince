#!/usr/bin/env bash
# The docs-describing check's own test: which pages it asks about, and which
# answers it accepts.
#
# Usage: bash scripts/check-docs-describing.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK="$SCRIPT_DIR/check-docs-describing.sh"
FIXTURE="$(mktemp -d)"
trap 'rm -rf "$FIXTURE"' EXIT

mkdir -p "$FIXTURE/docs/explanation" "$FIXTURE/docs/handbook"
printf '%s\n' '<!-- prose:plain -->' '<!-- describes: backend/internal/modules/comms frontend/src/app.tsx -->' \
	'# Sending' > "$FIXTURE/docs/explanation/sending.md"
printf '%s\n' '# Capture' '<!-- describes: backend/internal/modules/capture -->' \
	> "$FIXTURE/docs/handbook/capture.md"

FAILURES=0

# expect <want-exit> <want-in-output> <changed> <body|-unset-> <reason>
expect() {
	local want="$1" needle="$2" changed="$3" body="$4" reason="$5" out status=0
	if [[ "$body" == "-unset-" ]]; then
		out="$(DOCS_DESCRIBING_ROOT="$FIXTURE" DOCS_DESCRIBING_CHANGED="$changed" bash "$CHECK" 2>&1)" || status=$?
	else
		out="$(DOCS_DESCRIBING_ROOT="$FIXTURE" DOCS_DESCRIBING_CHANGED="$changed" DOCS_DESCRIBING_BODY="$body" \
			bash "$CHECK" 2>&1)" || status=$?
	fi
	if [[ "$status" -ne "$want" || "$out" != *"$needle"* ]]; then
		printf 'FAIL: %s\n  want exit %s with %q, got exit %s:\n%s\n' "$reason" "$want" "$needle" "$status" "$out"
		FAILURES=$((FAILURES + 1))
	fi
}

comms=backend/internal/modules/comms/dispatcher.go

expect 1 "unanswered  docs/explanation/sending.md" "$comms" "" \
	"a change under a described folder, with no answer, is asked about"
expect 0 "answered    docs/explanation/sending.md" "$comms" "- docs/explanation/sending.md: still true" \
	"a still-true answer is accepted"
expect 0 "answered" "$comms" '* `docs/explanation/sending.md`: Updated' \
	"an answer in backticks with a capital is accepted"
expect 1 "unanswered" "$comms" "I checked docs/explanation/sending.md and it still holds" \
	"prose that mentions the page is not an answer"
expect 0 "updated     docs/explanation/sending.md" "$comms
docs/explanation/sending.md" "" \
	"a page the change edits counts as answered"
expect 0 "ask         docs/explanation/sending.md" "frontend/src/app.tsx" "-unset-" \
	"list mode names the page and never fails"
expect 0 "no explanation or handbook page" "backend/internal/modules/commsx/a.go" "" \
	"a folder that only shares a prefix is not the described one"
expect 0 "no explanation or handbook page" "backend/internal/modules/capture/x.go" "" \
	"a header under the title is not read"

if ((FAILURES > 0)); then
	echo "$FAILURES case(s) failed"
	exit 1
fi
echo "check-docs-describing: all cases pass"
