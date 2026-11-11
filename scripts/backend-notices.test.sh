#!/usr/bin/env bash
# The notice generator's own test: what it writes, and what it refuses.
#
# Each case plants a small corpus through the generator's documented overrides
# and judges the file. One case runs the real `go list` corpus, because a
# generator that works only on planted modules proves nothing about the build.
#
# Usage: bash scripts/backend-notices.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GENERATOR="$SCRIPT_DIR/backend-notices.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
FAILURES=0

fail() {
	printf 'FAIL: %s\n' "$1"
	FAILURES=$((FAILURES + 1))
}

mkdir -p "$work/goroot" "$work/mod/licensed" "$work/mod/noticed" "$work/mod/bare"
echo 'GO-TOOLCHAIN-LICENSE-TEXT' >"$work/goroot/LICENSE"
echo 'LICENSED-MODULE-TEXT' >"$work/mod/licensed/LICENSE"
echo 'NOTICED-MODULE-LICENSE' >"$work/mod/noticed/COPYING"
echo 'NOTICED-MODULE-NOTICE' >"$work/mod/noticed/NOTICE.txt"
echo 'only a readme' >"$work/mod/bare/README.md"
printf 'example.com/embedded | blob.bin | a vendored blob | MIT\n' >"$work/declared.txt"

generate() {
	BACKEND_NOTICES_MODULES="$1" BACKEND_NOTICES_GOROOT="$work/goroot" \
		BACKEND_NOTICES_DECLARED="$work/declared.txt" \
		sh "$GENERATOR" "$work/out/notices.txt" >"$work/log" 2>&1
}

# --- a complete corpus ---------------------------------------------------------
printf '%s\n' \
	"example.com/licensed|v1.0.0|$work/mod/licensed" \
	"example.com/noticed|v2.1.0|$work/mod/noticed" >"$work/complete"
if ! generate "$work/complete"; then
	fail "a corpus with licence text for every module was refused: $(cat "$work/log")"
else
	for want in GO-TOOLCHAIN-LICENSE-TEXT LICENSED-MODULE-TEXT NOTICED-MODULE-LICENSE NOTICED-MODULE-NOTICE \
		'example.com/licensed v1.0.0' 'example.com/embedded | blob.bin'; do
		grep -qF "$want" "$work/out/notices.txt" || fail "the notice file lacks: $want"
	done
fi

# --- a module with no licence text -------------------------------------------
printf '%s\n' "example.com/bare|v0.3.0|$work/mod/bare" >"$work/missing"
if generate "$work/missing"; then
	fail "a module with no licence text was accepted, and the file would claim a completeness it lacks"
elif ! grep -q 'example.com/bare' "$work/log"; then
	fail "the refusal does not name the module: $(cat "$work/log")"
fi

# --- the repository's own modules are not third party --------------------------
printf '%s\n' "example.com/licensed|v1.0.0|$work/mod/licensed" \
	"github.com/margince/margince/backend||$SCRIPT_DIR/../backend" >"$work/own"
if ! generate "$work/own"; then
	fail "the repository's own module was held to the third-party rule: $(cat "$work/log")"
elif grep -q 'margince/backend' <(sed -n '1,/^Embedded binaries$/p' "$work/out/notices.txt"); then
	fail "the repository's own module was listed as a third-party notice"
fi

# --- the real corpus -----------------------------------------------------------
if ! sh "$GENERATOR" "$work/real.txt" >"$work/log" 2>&1; then
	fail "the real corpus was refused: $(cat "$work/log")"
else
	for want in github.com/jackc/pgx/v5 golang.org/x/net 'pdfium.wasm' 'Go toolchain'; do
		grep -qF "$want" "$work/real.txt" || fail "the real notice file lacks: $want"
	done
fi

if [[ "$FAILURES" -gt 0 ]]; then
	exit 1
fi
echo "OK: the notice file carries every linked module's licence text and refuses a module without one"
