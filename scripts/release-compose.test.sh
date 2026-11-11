#!/usr/bin/env bash
# The compose packer's own test: what the folder holds, and what it refuses.
#
# Its one caller is a tag push, so without this the first look at the folder is
# somebody's host. Each case writes an images.txt and judges the tarball.
#
# Usage: bash scripts/release-compose.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PACKER="$SCRIPT_DIR/release-compose.sh"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
FAILURES=0

fail() {
	printf 'FAIL: %s\n' "$1"
	FAILURES=$((FAILURES + 1))
}

digest() { printf 'sha256:%064d' "$1"; }

# images <role>...: an images.txt naming each role by a digest of its own.
images() {
	local n=0
	for role in "$@"; do
		n=$((n + 1))
		echo "ghcr.io/margince/$role:v1.2.0@$(digest "$n")"
	done >"$work/images.txt"
}

pack() {
	rm -rf "$work/release"
	bash "$PACKER" v1.2.0 "$work/images.txt" "$work/release" >"$work/out" 2>&1
}

# --- every image pushed -------------------------------------------------------
images api web worker
if ! pack; then
	fail "a complete images.txt was refused: $(cat "$work/out")"
fi
tarball="$work/release/margince-v1.2.0-compose.tar.gz"
mkdir -p "$work/unpacked"
tar -xzf "$tarball" -C "$work/unpacked"
top="$work/unpacked/margince-v1.2.0-compose"
for file in compose.yaml .env.example db-init.sh db-bootstrap.sql front.conf proxy.conf margince.yaml; do
	if [[ ! -s "$top/$file" ]]; then
		fail "the folder has no $file, which the stack mounts or the operator starts from"
	fi
done
for want in "ghcr.io/margince/api:v1.2.0@$(digest 1)" "ghcr.io/margince/web:v1.2.0@$(digest 2)" "ghcr.io/margince/worker:v1.2.0@$(digest 3)"; do
	if ! grep -qxF "    image: $want" "$top/compose.yaml"; then
		fail "compose.yaml does not pin $want, so a host could pull another build"
	fi
done
if grep -q ':RELEASE' "$top/compose.yaml"; then
	fail "compose.yaml still names the RELEASE placeholder, which pulls nothing"
fi
if ! grep -qxF '    server api:8080;' "$top/front.conf" || ! grep -qxF '    server web:8080;' "$top/front.conf"; then
	fail "front.conf still sends requests to 127.0.0.1, where no service listens in the stack"
fi
if ! grep -qxF '    listen 8080;' "$top/front.conf"; then
	fail "front.conf listens on a port the unprivileged nginx cannot bind"
fi
if [[ "$(grep -c 'location' "$top/front.conf")" != "$(grep -c 'location' "$SCRIPT_DIR/deploy/server/nginx.conf")" ]]; then
	fail "front.conf lost routes the server bundle's nginx.conf sends to the api"
fi
if ! grep -qxF '  password_file: secrets/admin-password' "$top/margince.yaml"; then
	fail "margince.yaml reads the admin password from a file the api entrypoint never writes"
fi
if [[ ! -x "$top/db-init.sh" ]]; then
	fail "db-init.sh is not executable, so PostgreSQL would source it instead of running it"
fi

# --- a role that was not pushed ----------------------------------------------
images api web
if pack; then
	fail "an images.txt without the worker was packed, and the stack would pull nothing for it"
elif ! grep -q 'worker' "$work/out"; then
	fail "the refusal does not name the missing image: $(cat "$work/out")"
fi

# --- a role named by tag alone -----------------------------------------------
images api web worker
sed -i.bak 's#^\(ghcr.io/margince/web:v1.2.0\)@.*#\1#' "$work/images.txt"
if pack; then
	fail "an image with no digest was packed, and a moved tag would change what the stack runs"
fi

if [[ "$FAILURES" -gt 0 ]]; then
	exit 1
fi
echo "OK: the compose folder pins every image and mounts the server bundle's routes"
