#!/usr/bin/env bash
# ci-image-digest.sh <ghcr.io/owner/name:tag> — print `digest=<sha256:…>` when
# an anonymous client can pull that image, and `digest=` when it cannot.
#
# Anonymous on purpose: a job's `container:` carries no credentials, so an image
# that needs a login is one no job can start in. "Cannot" covers a tag nobody
# pushed and a package that is not public; the registry answers both with a
# refusal, and both send the jobs to the runner. Any other answer is a registry
# fault, which fails the step after a retry rather than pass as a missing image
# and start a rebuild.
set -euo pipefail
ref="${1:?usage: ci-image-digest.sh <ghcr.io/owner/name:tag>}"
case "$ref" in
	ghcr.io/*:*) ;;
	*) echo "ci-image-digest: '$ref' is not a ghcr.io name with a tag" >&2; exit 2 ;;
esac
name="${ref#ghcr.io/}"
tag="${name##*:}"
name="${name%:*}"

# status <url> [header…] — print the HTTP status, with the body in $body and
# the headers in $headers. Network failures and 5xx are retried by curl.
status() {
	local url="$1"
	shift
	curl -sS --retry 3 --retry-all-errors --max-time 30 \
		-o "$body" -D "$headers" -w '%{http_code}' "$@" "$url"
}

body="$(mktemp)"
headers="$(mktemp)"
trap 'rm -f "$body" "$headers"' EXIT

code="$(status "https://ghcr.io/token?scope=repository:$name:pull")"
case "$code" in
	200) ;;
	401 | 403) echo "digest="; exit 0 ;;
	*) echo "ci-image-digest: the registry's token endpoint answered $code" >&2; exit 1 ;;
esac
token="$(jq -r '.token // empty' "$body")"
[[ -n "$token" ]] || { echo "ci-image-digest: the token answer carried no token" >&2; exit 1; }

code="$(status "https://ghcr.io/v2/$name/manifests/$tag" -I \
	-H "Authorization: Bearer $token" \
	-H 'Accept: application/vnd.oci.image.index.v1+json' \
	-H 'Accept: application/vnd.oci.image.manifest.v1+json' \
	-H 'Accept: application/vnd.docker.distribution.manifest.list.v2+json' \
	-H 'Accept: application/vnd.docker.distribution.manifest.v2+json')"
case "$code" in
	200) ;;
	401 | 403 | 404) echo "digest="; exit 0 ;;
	*) echo "ci-image-digest: the registry answered $code for $ref" >&2; exit 1 ;;
esac
digest="$(tr -d '\r' <"$headers" | awk 'tolower($1) == "docker-content-digest:" { print $2 }')"
if ! [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
	echo "ci-image-digest: $ref answered without a sha256 digest ('$digest')" >&2
	exit 1
fi
echo "digest=$digest"
