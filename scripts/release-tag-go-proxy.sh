#!/usr/bin/env bash
# The tag names the commit Go recorded for it, or Go recorded nothing yet.
#
# A v tag is also a version of the root Go module. The first time anyone asks
# the Go module proxy for it (go get, pkg.go.dev, a client's update), the proxy
# records the commit the tag names, and the checksum database records its
# contents. Neither changes again. A tag deleted and pushed again on another
# commit then makes a release whose source is not the Go version of the same
# name: the proxy serves the old commit, and a download from git fails Go's
# checksum check with "SECURITY ERROR".
#
# The question goes to the proxy's cached-only view, which answers from what
# is recorded and fetches nothing, so the check does not record the version
# itself. A tag deleted and tagged again after a failed run still passes while
# nothing asked for it.
#
# Three answers:
#   not recorded       — passes.
#   the same commit    — passes: a run of the same tag again.
#   another commit     — REFUSES: the version is spent; tag the next one.
# Any other answer refuses too: a check that cannot run must not pass.
#
# WHAT IT READS: the module path in ./go.mod, and the proxy over HTTPS.
# GO_PROXY_CACHED overrides the proxy for the test.
#
# Usage: bash scripts/release-tag-go-proxy.sh v1.0.0 <commit>
set -euo pipefail

tag="${1:-}"
commit="${2:-}"
if [[ -z "$tag" || -z "$commit" ]]; then
	echo "release-tag-go-proxy: pass the tag and the commit it names." >&2
	exit 1
fi
proxy="${GO_PROXY_CACHED:-https://proxy.golang.org/cached-only}"
module="$(awk '$1 == "module" { print $2; exit }' go.mod)"
if [[ -z "$module" ]]; then
	echo "release-tag-go-proxy: ./go.mod names no module." >&2
	exit 1
fi

body="$(mktemp)"
trap 'rm -f "$body"' EXIT
status="$(curl --silent --show-error --max-time 30 --retry 3 --retry-all-errors \
	--output "$body" --write-out '%{http_code}' "$proxy/$module/@v/$tag.info" 2>&1)" || {
	echo "release-tag-go-proxy: the Go module proxy did not answer: $status" >&2
	echo "  Run all jobs again once it answers." >&2
	exit 1
}

case "$status" in
	404 | 410)
		echo "release-tag-go-proxy: Go has not recorded $module@$tag yet."
		exit 0
		;;
	200) ;;
	*)
		echo "release-tag-go-proxy: the Go module proxy answered $status for $module@$tag." >&2
		echo "  Run all jobs again once it answers." >&2
		exit 1
		;;
esac

recorded="$(jq -r '.Origin.Hash // empty' "$body")"
if [[ -z "$recorded" ]]; then
	echo "release-tag-go-proxy: Go recorded $module@$tag but names no commit for it." >&2
	echo "  The check cannot tell whether $tag moved, so it refuses." >&2
	exit 1
fi
if [[ "$recorded" != "$commit" ]]; then
	echo "release-tag-go-proxy: Go recorded $module@$tag as commit $recorded," >&2
	echo "  but $tag now names $commit. Go never changes a recorded version, so" >&2
	echo "  this version is spent. Tag the next version instead:" >&2
	echo "  docs/how-to/cut-a-release.md#when-a-run-fails." >&2
	exit 1
fi
echo "release-tag-go-proxy: Go recorded $module@$tag at this commit."
