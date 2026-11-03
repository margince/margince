#!/usr/bin/env bash
# The patch base adapter's own test: the records the release CLI's
# resolve-patch-base writes, and the records it must refuse.
#
# A refused record exits 1 and prints nothing on stdout, because the workflow
# reads stdout as shell values and an empty answer must never read as a base.
#
# Usage: bash scripts/release-patch-base.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ADAPT="$SCRIPT_DIR/release-patch-base.sh"

FAILURES=0
SHA1="3f786850e387550fdab836ed7e6dc881de23001b"
SHA256="9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

# expect <name> <want-stdout> <stdin>
expect() {
	local name="$1" want="$2" got
	if ! got="$(printf '%s' "$3" | bash "$ADAPT" 2>/dev/null)"; then
		printf 'FAIL: %s — refused a record it must accept\n' "$name"
		FAILURES=$((FAILURES + 1))
		return
	fi
	if [[ "$got" != "$want" ]]; then
		printf 'FAIL: %s — printed\n%s\nwant\n%s\n' "$name" "$got" "$want"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    %s\n' "$name"
}

# refuse <name> <stdin>
refuse() {
	local name="$1" got status=0
	got="$(printf '%s' "$2" | bash "$ADAPT" 2>/dev/null)" || status=$?
	if [[ "$status" -ne 1 || -n "$got" ]]; then
		printf 'FAIL: %s — exit %d with stdout %q, want exit 1 and no stdout\n' "$name" "$status" "$got"
		FAILURES=$((FAILURES + 1))
		return
	fi
	printf 'ok    %s\n' "$name"
}

expect "a resolved base gives its version and revision" \
	"$(printf 'first_release=false\nbase_version=1970.41.0\nbase_revision=%s' "$SHA1")" \
	"{\"event\":\"patch-base-resolved\",\"version\":\"1970.41.0\",\"revision\":\"$SHA1\"}
"

expect "a SHA-256 revision and a patch level are a base too" \
	"$(printf 'first_release=false\nbase_version=2028.0.3-p2\nbase_revision=%s' "$SHA256")" \
	"{\"event\":\"patch-base-resolved\",\"version\":\"2028.0.3-p2\",\"revision\":\"$SHA256\"}
"

expect "no release went out: the first release" \
	"first_release=true" \
	'{"event":"first-release"}
'

refuse "the CLI's error record is no answer" \
	'{"event":"error","message":"read the release list: dist answered 401 Unauthorized"}
'
refuse "an unknown event is no answer" \
	"{\"event\":\"patch-base-guessed\",\"version\":\"1970.41.0\",\"revision\":\"$SHA1\"}
"
refuse "a base without a revision" \
	'{"event":"patch-base-resolved","version":"1970.41.0"}
'
refuse "a base without a version" \
	"{\"event\":\"patch-base-resolved\",\"revision\":\"$SHA1\"}
"
refuse "a malformed version" \
	"{\"event\":\"patch-base-resolved\",\"version\":\"v1970.41\",\"revision\":\"$SHA1\"}
"
refuse "a version that carries shell" \
	"{\"event\":\"patch-base-resolved\",\"version\":\"1970.41.0\$(id)\",\"revision\":\"$SHA1\"}
"
refuse "an abbreviated revision" \
	'{"event":"patch-base-resolved","version":"1970.41.0","revision":"3f78685"}
'
refuse "a revision that is a ref name" \
	'{"event":"patch-base-resolved","version":"1970.41.0","revision":"main"}
'
refuse "two records" \
	"{\"event\":\"first-release\"}
{\"event\":\"patch-base-resolved\",\"version\":\"1970.41.0\",\"revision\":\"$SHA1\"}
"
refuse "no record at all" ""
refuse "a line that is not JSON" 'patch base 1970.41.0
'

if [[ "$FAILURES" -ne 0 ]]; then
	printf '\nrelease-patch-base: %d case(s) failed\n' "$FAILURES" >&2
	exit 1
fi
echo "OK: release-patch-base — every record the release CLI writes, and every one it must refuse"
