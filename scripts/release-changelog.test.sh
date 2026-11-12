#!/usr/bin/env bash
# The dist changelog builder's own test: the changelog each release carries,
# and the calls it must refuse.
#
# A refused call exits 1 and prints nothing on stdout, because the workflow
# writes stdout to the file it uploads, and an empty file must never go out as
# a changelog.
#
# Usage: bash scripts/release-changelog.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD="$SCRIPT_DIR/release-changelog.sh"

FAILURES=0
AFTER="9f86d081884c7d659a2feaa0c55ad015a3bf4f1b"
BASE="3f786850e387550fdab836ed7e6dc881de23001b"

# Each call reads its stdin from a process substitution, not a pipe: a call
# that exits before it reads stdin would kill a writing pipe with SIGPIPE, and
# pipefail would turn that into a failed case.

# expect <name> <want-stdout> <stdin> <args...>
expect() {
	local name="$1" want="$2" input="$3" got
	shift 3
	if ! got="$(bash "$BUILD" "$@" 2>/dev/null < <(printf '%s' "$input"))"; then
		printf 'FAIL: %s — refused a call it must accept\n' "$name"
		FAILURES=$((FAILURES + 1))
		return
	fi
	if [[ "$got" != "$want" ]]; then
		# The first 2000 characters: the room cases compare a quarter megabyte.
		printf 'FAIL: %s\n--- want\n%s\n--- got\n%s\n' "$name" "${want:0:2000}" "${got:0:2000}"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    %s\n' "$name"
}

# refuse <name> <stdin> <args...>
refuse() {
	local name="$1" input="$2" got status=0
	shift 2
	got="$(bash "$BUILD" "$@" 2>/dev/null < <(printf '%s' "$input"))" || status=$?
	if [[ "$status" -ne 1 || -n "$got" ]]; then
		printf 'FAIL: %s — exit %d with stdout %q, want exit 1 and no stdout\n' "$name" "$status" "$got"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    %s\n' "$name"
}

compare="Every change since 2026.1.0: https://github.com/margince/margince/compare/$BASE...$AFTER"

expect "a base with subjects lists them under the heading" "$(
	cat <<EOF
## Changes

### Breaking

- **api:** offers drop the legacy total (#12)

### Added

- **deals:** a deal shows its next step (#10)

### Fixed

- **sign-in:** a link opened before sign-in opens after it (#11)

$compare
EOF
)" 'feat(deals): a deal shows its next step (#10)
fix(sign-in): a link opened before sign-in opens after it (#11)
feat(api)!: offers drop the legacy total (#12)
chore(deps): update a pin (#13)
' 2026.1.1 margince/margince "$AFTER" 2026.1.0 "$BASE"

expect "subjects no user notices give only the compare line" "$compare" 'chore(deps): update a pin (#13)
Update dependency happy-dom to v20.14.6 (#14)
' 2026.1.1 margince/margince "$AFTER" 2026.1.0 "$BASE"

expect "the first release gives one sentence and reads no subjects" \
	"The first release on this line: the patch is the whole tree." 'feat(deals): a deal shows its next step (#10)
' 2026.1.0 margince/margince "$AFTER"

expect "an invalid UTF-8 byte in a subject is dropped, not refused" "$(
	cat <<EOF
## Changes

### Fixed

- **offers:** a caf total rounds to the cent (#15)

$compare
EOF
)" $'fix(offers): a caf\xe9 total rounds to the cent (#15)\n' \
	2026.1.1 margince/margince "$AFTER" 2026.1.0 "$BASE"

# The room: release-notes.sh lists every fix up to 250000 characters and
# collapses them over it. 2400 fixes of 100 characters fit it, 2600 do not.
fixes() {
	local i
	for ((i = 1; i <= $1; i++)); do
		printf 'fix: %s (#%05d)\n' "$long" "$i"
	done
}
listed() {
	local i
	printf '## Changes\n\n### Fixed\n\n'
	for ((i = 1; i <= $1; i++)); do
		printf -- '- %s (#%05d)\n' "$long" "$i"
	done
	printf '\n%s\n' "$compare"
}
long="$(printf '%0090d' 0 | tr 0 x)"
expect "fixes inside the room are listed in full" "$(listed 2400)" "$(fixes 2400)" \
	2026.1.1 margince/margince "$AFTER" 2026.1.0 "$BASE"
expect "fixes over the room collapse into a count" "$(
	cat <<EOF
## Changes

### Fixed

- 2600 more, listed in the compare view.

$compare
EOF
)" "$(fixes 2600)" 2026.1.1 margince/margince "$AFTER" 2026.1.0 "$BASE"

refuse "a base with no subject since it" "" \
	2026.1.1 margince/margince "$AFTER" 2026.1.0 "$BASE"
refuse "a base with only empty lines since it" $'\n\n' \
	2026.1.1 margince/margince "$AFTER" 2026.1.0 "$BASE"
refuse "a base version without its revision" 'feat: a thing (#1)
' 2026.1.1 margince/margince "$AFTER" 2026.1.0
refuse "no revision" "" 2026.1.0 margince/margince
refuse "no arguments at all" ""
refuse "an empty version" "" "" margince/margince "$AFTER"
refuse "an empty base revision" 'feat: a thing (#1)
' 2026.1.1 margince/margince "$AFTER" 2026.1.0 ""
long="$(printf '%0100d' 0 | tr 0 x)"
refuse "breaking changes that cannot fit the room" "$(
	for i in $(seq 1 3000); do
		printf 'feat(api)!: %s (#%d)\n' "$long" "$i"
	done
)" 2026.1.1 margince/margince "$AFTER" 2026.1.0 "$BASE"

if [[ "$FAILURES" -ne 0 ]]; then
	printf '\nrelease-changelog: %d case(s) failed\n' "$FAILURES" >&2
	exit 1
fi
echo "OK: release-changelog — the changelog of a release with a base, of a first release, and every call it refuses"
