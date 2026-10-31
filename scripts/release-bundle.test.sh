#!/usr/bin/env bash
# The bundle packer's own test: what each tarball holds, and what it refuses.
#
# Its one caller is a tag push, so without this the first look at a bundle is
# somebody's server. Each case plants the folders the bake exports, with stub
# programs, and judges the tarballs.
#
# Usage: bash scripts/release-bundle.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PACKER="$SCRIPT_DIR/release-bundle.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
FAILURES=0

fail() {
	printf 'FAIL: %s\n' "$1"
	FAILURES=$((FAILURES + 1))
}

# plant <arch>: one platform folder, laid out as the Dockerfile's bundle stage.
plant() {
	local dir="$work/bundle/linux_$1"
	mkdir -p "$dir/bin" "$dir/web" "$dir/deploy/server"
	for program in margince-api margince-worker margince-migrate; do
		printf '#!/bin/sh\necho %s-%s\n' "$program" "$1" >"$dir/bin/$program"
	done
	echo '<!doctype html>' >"$dir/web/index.html"
	cp "$SCRIPT_DIR"/deploy/*.sh "$SCRIPT_DIR"/deploy/*.sql "$dir/deploy/"
	cp "$SCRIPT_DIR"/deploy/server/* "$dir/deploy/server/"
	echo 'Business Source License 1.1' >"$dir/LICENSE"
}

pack() {
	rm -rf "$work/release"
	bash "$PACKER" v1.2.0 4e1b736eb "$work/bundle" "$work/release" >"$work/out" 2>&1
}

# --- a complete bake ---------------------------------------------------------
plant amd64
plant arm64
if ! pack; then
	fail "a complete bake was refused: $(cat "$work/out")"
fi
for arch in amd64 arm64; do
	tarball="$work/release/margince-v1.2.0-linux-$arch.tar.gz"
	if [[ ! -s "$tarball" ]]; then
		fail "no tarball for linux/$arch, so the release page would offer no bundle for it"
		continue
	fi
	rm -rf "$work/unpacked"
	mkdir -p "$work/unpacked"
	tar -xzf "$tarball" -C "$work/unpacked"
	top="$work/unpacked/margince-v1.2.0-linux-$arch"
	if [[ "$(cat "$top/VERSION" 2>/dev/null)" != $'version=v1.2.0\nrevision=4e1b736eb' ]]; then
		fail "linux/$arch VERSION does not name the release and the revision, so the files alone cannot say which release they are"
	fi
	if [[ "$("$top/bin/margince-api" 2>/dev/null)" != "margince-api-$arch" ]]; then
		fail "linux/$arch carries another platform's api, or one that is not executable"
	fi
	conf="$top/deploy/server/web.conf"
	if ! grep -q '^    listen 127.0.0.1:8081;$' "$conf" || grep -q 'listen 8080;' "$conf"; then
		fail "linux/$arch web.conf still listens on 8080, the port the api takes on a server"
	fi
	if ! grep -q '^    root /opt/margince/web;$' "$conf"; then
		fail "linux/$arch web.conf does not serve the bundle's web/ folder"
	fi
	if [[ "$(grep -c 'add_header' "$conf")" != "$(grep -c 'add_header' "$SCRIPT_DIR/../frontend/nginx.conf")" ]]; then
		fail "linux/$arch web.conf lost headers the web image sends"
	fi
done

# --- a bake that exported one platform -------------------------------------
rm -rf "$work/bundle/linux_arm64"
if pack; then
	fail "a bake with no linux/arm64 folder was packed, and the page would offer one bundle"
elif ! grep -q 'linux/arm64' "$work/out"; then
	fail "the refusal does not name the missing platform: $(cat "$work/out")"
fi

# --- a bundle without a program the units start ------------------------------
plant arm64
rm "$work/bundle/linux_arm64/bin/margince-worker"
if pack; then
	fail "a bundle without margince-worker was packed, and its unit would fail on the server"
elif ! grep -q 'bin/margince-worker' "$work/out"; then
	fail "the refusal does not name the missing file: $(cat "$work/out")"
fi

if [[ "$FAILURES" -gt 0 ]]; then
	exit 1
fi
echo "OK: each Linux bundle carries its release, its programs and a web server off the api's port"
