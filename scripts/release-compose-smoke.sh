#!/usr/bin/env bash
# Start a release's compose folder as an operator would and wait for it to serve.
#
# The release job runs it after the images are pushed, so a folder that packs
# but does not start fails the release instead of reaching a host. It answers
# through the front server: readiness, the anonymous status route, the web app.
# It runs as production with no license token, so it starts on its trial as a
# new install does.
#
# Usage: bash scripts/release-compose-smoke.sh dist/release/margince-v1.0.0-compose.tar.gz
set -euo pipefail

tarball="${1:-}"
if [[ ! -s "$tarball" ]]; then
	echo "release-compose-smoke: pass the compose tarball the release packed." >&2
	exit 1
fi
port="${SMOKE_PORT:-18480}"
work="$(mktemp -d)"
tar -xzf "$tarball" -C "$work"
dir="$(find "$work" -mindepth 1 -maxdepth 1 -type d)"
cd "$dir"

cleanup() {
	docker compose down -v --remove-orphans >/dev/null 2>&1 || true
	rm -rf "$work"
}
trap cleanup EXIT

{
	echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)"
	echo "MARGINCE_OWNER_PW=$(openssl rand -hex 24)"
	echo "MARGINCE_APP_PW=$(openssl rand -hex 24)"
	echo "MARGINCE_PUBLIC_BASE_URL=http://localhost:$port"
	echo "MARGINCE_KEYVAULT_ROOT_KEY=$(openssl rand -base64 32)"
	echo "MARGINCE_ADMIN_PASSWORD=$(openssl rand -hex 24)"
	echo "MARGINCE_PORT=$port"
} >.env

docker compose up -d --quiet-pull

# expect <path> <code>: the front server answers path with code.
expect() {
	local got
	got="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$port$1" || true)"
	[[ "$got" == "$2" ]]
}

for _ in $(seq 90); do
	expect /readyz 200 && break
	sleep 2
done
failed=0
for check in "/readyz 200" "/v1/status 200" "/ 200" "/setup/status 200"; do
	read -r path code <<<"$check"
	if ! expect "$path" "$code"; then
		echo "release-compose-smoke: $path did not answer $code through the front server." >&2
		failed=1
	fi
done
if [[ "$failed" == 1 ]]; then
	docker compose ps -a >&2
	docker compose logs --no-color --tail 60 api worker front >&2
	exit 1
fi
echo "release-compose-smoke: the compose folder starts and serves through its front server"
