#!/usr/bin/env bash
# Pack the server bundles of one release, one tarball per Linux platform.
#
# The input is what `docker buildx bake bundle` exports: one folder per platform
# (linux_amd64/, linux_arm64/), each the Dockerfile's `bundle` stage. Each
# tarball adds two things the stage cannot hold:
#
#   VERSION                 the release and the commit, for a reader who has the
#                           files and nothing else
#   deploy/server/web.conf  the web image's own nginx.conf, moved off the port
#                           the api listens on and onto the bundle's web/ folder
#
# A platform folder that is missing, or one without a program the units start,
# is refused: a release page that offers a bundle with nothing to run is worse
# than a release that failed in seconds.
#
# WHAT IT READS: its arguments and frontend/nginx.conf. No git, no network, so
# its own test can plant a bundle and judge the tarballs.
#
# Usage: bash scripts/release-bundle.sh v1.0.0 <revision> dist/bundle dist/release
set -euo pipefail

version="${1:-}"
revision="${2:-}"
bundles="${3:-}"
out="${4:-}"
if [[ -z "$version" || -z "$revision" || -z "$bundles" || -z "$out" ]]; then
	echo "release-bundle: pass the version, the revision, the bake output and the output folder." >&2
	exit 1
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
nginx_conf="$root/frontend/nginx.conf"

# Every file a server install starts from. A bundle without one of them would
# pack and upload cleanly and fail only on somebody's server.
required=(
	bin/margince-api
	bin/margince-worker
	bin/margince-migrate
	web/index.html
	deploy/api-entrypoint.sh
	deploy/worker-entrypoint.sh
	deploy/db-bootstrap.sql
	deploy/server/nginx.conf
	deploy/server/proxy.conf
	deploy/server/margince-api.service
	deploy/server/margince-worker.service
	LICENSE
)

# The web server listens where the front server's margince_web upstream points,
# and serves the folder the install guide unpacks the bundle into.
web_conf() {
	local listen root
	listen="$(grep -c '^    listen 8080;$' "$nginx_conf" || true)"
	root="$(grep -c '^    root /usr/share/nginx/html;$' "$nginx_conf" || true)"
	if [[ "$listen" != 1 || "$root" != 1 ]]; then
		echo "release-bundle: frontend/nginx.conf no longer has one 'listen 8080;' and one" >&2
		echo "  'root /usr/share/nginx/html;' in its server block, so web.conf cannot be derived." >&2
		echo "  Update web_conf() in this script to match it." >&2
		exit 1
	fi
	sed -e 's#^    listen 8080;$#    listen 127.0.0.1:8081;#' \
		-e 's#^    root /usr/share/nginx/html;$#    root /opt/margince/web;#' "$nginx_conf"
}

mkdir -p "$out"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

for arch in amd64 arm64; do
	from="$bundles/linux_$arch"
	if [[ ! -d "$from" ]]; then
		echo "release-bundle: $from does not exist; the bake exported no linux/$arch bundle." >&2
		exit 1
	fi
	for file in "${required[@]}"; do
		if [[ ! -s "$from/$file" ]]; then
			echo "release-bundle: the linux/$arch bundle has no $file." >&2
			exit 1
		fi
	done

	name="margince-$version-linux-$arch"
	mkdir -p "$work/$name"
	cp -R "$from/." "$work/$name/"
	chmod +x "$work/$name"/bin/* "$work/$name"/deploy/*.sh
	web_conf >"$work/$name/deploy/server/web.conf"
	printf 'version=%s\nrevision=%s\n' "$version" "$revision" >"$work/$name/VERSION"
	tar -czf "$out/$name.tar.gz" -C "$work" "$name"
	echo "release-bundle: packed $out/$name.tar.gz"
done
