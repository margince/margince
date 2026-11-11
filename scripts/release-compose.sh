#!/usr/bin/env bash
# Pack the Docker Compose folder of one release: margince-<version>-compose.tar.gz.
#
# It holds scripts/deploy/compose/ with each Margince image pinned to the digest
# images.txt names, plus what the stack mounts, each taken from the file the
# server bundle already ships:
#
#   front.conf       scripts/deploy/server/nginx.conf, sent to the api and web
#                    services and moved to the unprivileged port
#   proxy.conf       scripts/deploy/server/proxy.conf, as it is
#   db-bootstrap.sql scripts/deploy/db-bootstrap.sql, as it is
#   margince.yaml    config/margince.example.yaml, with the admin password file
#                    where the api entrypoint writes it
#
# A role missing from images.txt, or a line this script no longer finds in a
# source file, is refused: the folder would pack and fail only on a host.
#
# WHAT IT READS: its arguments and the files above. No git, no network.
#
# Usage: bash scripts/release-compose.sh v1.0.0 dist/release/images.txt dist/release
set -euo pipefail

version="${1:-}"
images="${2:-}"
out="${3:-}"
if [[ -z "$version" || -z "$images" || -z "$out" ]]; then
	echo "release-compose: pass the version, images.txt and the output folder." >&2
	exit 1
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
compose_dir="$root/scripts/deploy/compose"
server_dir="$root/scripts/deploy/server"

# replace_once <file> <from> <to>: the line must be there exactly once, or the
# source changed under this script and the output would be wrong.
replace_once() {
	local file="$1" from="$2" to="$3" count
	count="$(grep -cxF -- "$from" "$file" || true)"
	if [[ "$count" != 1 ]]; then
		echo "release-compose: expected one line '$from' in $(basename "$file"), found $count." >&2
		echo "  Update scripts/release-compose.sh to match it." >&2
		exit 1
	fi
	FROM="$from" TO="$to" perl -0pi -e 's/^\Q$ENV{FROM}\E$/$ENV{TO}/m' "$file"
}

name="margince-$version-compose"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
dir="$work/$name"
mkdir -p "$dir" "$out"

cp "$compose_dir/compose.yaml" "$dir/compose.yaml"
for role in api web worker; do
	ref="$(grep -E "^ghcr\.io/margince/$role:[^@]+@sha256:[0-9a-f]{64}$" "$images" || true)"
	if [[ -z "$ref" || "$(printf '%s\n' "$ref" | wc -l)" -ne 1 ]]; then
		echo "release-compose: $images names no single digest for the $role image." >&2
		exit 1
	fi
	replace_once "$dir/compose.yaml" "    image: ghcr.io/margince/$role:RELEASE" "    image: $ref"
done

cp "$compose_dir/env.example" "$dir/.env.example"
cp "$compose_dir/db-init.sh" "$dir/db-init.sh"
chmod +x "$dir/db-init.sh"
cp "$root/scripts/deploy/db-bootstrap.sql" "$dir/db-bootstrap.sql"
cp "$server_dir/proxy.conf" "$dir/proxy.conf"

cp "$server_dir/nginx.conf" "$dir/front.conf"
replace_once "$dir/front.conf" "    server 127.0.0.1:8080;" "    server api:8080;"
replace_once "$dir/front.conf" "    server 127.0.0.1:8081;" "    server web:8080;"
replace_once "$dir/front.conf" "    listen 80;" "    listen 8080;"
replace_once "$dir/front.conf" "    server_name margince.example.com;" "    server_name _;"

cp "$root/config/margince.example.yaml" "$dir/margince.yaml"
replace_once "$dir/margince.yaml" "  password_file: config/margince-admin-password" "  password_file: secrets/admin-password"

tar -czf "$out/$name.tar.gz" -C "$work" "$name"
echo "release-compose: packed $out/$name.tar.gz"
