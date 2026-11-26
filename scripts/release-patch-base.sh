#!/usr/bin/env bash
# The patch base of the next release, as shell values the release workflow reads.
#
# The release CLI's `resolve-patch-base --release <next> --jsonl` asks the dist
# service and writes one record: `patch-base-resolved` with the version and git
# revision of the newest release that went out, withdrawn included, or
# `first-release` when none went out. One dist row gives both values, so the
# patch is cut from the same release the upload names as its base.
#
# Usage: release-management resolve-patch-base --release <next> --jsonl | scripts/release-patch-base.sh
#
# Prints `first_release=true`, or `first_release=false`, `base_version=<v>` and
# `base_revision=<r>`, one per line. Any other record exits 1 and prints nothing
# on stdout: the workflow sources stdout, so only checked values reach it.
set -euo pipefail

# shellcheck source=scripts/lib-dist-version.sh
source "$(dirname "$0")/lib-dist-version.sh"

record="$(cat)"

refuse() {
	printf 'release-patch-base: %s\n  record: %s\n' "$1" "${record:-<none>}" >&2
	exit 1
}

if [[ -z "$record" || "$record" == *$'\n'* ]]; then
	refuse "want exactly one record from resolve-patch-base"
fi
if ! count="$(jq -s length <<<"$record" 2>/dev/null)" || [[ "$count" != 1 ]]; then
	refuse "the record is not one JSON value"
fi

field() {
	jq -r --arg name "$1" '.[$name] | strings' <<<"$record"
}

case "$(field event)" in
first-release)
	echo "first_release=true"
	;;
patch-base-resolved)
	version="$(field version)"
	revision="$(field revision)"
	if [[ ! "$version" =~ $DIST_VERSION_RE ]]; then
		refuse "the base version is not a release version"
	fi
	if [[ ! "$revision" =~ ^([0-9a-f]{40}|[0-9a-f]{64})$ ]]; then
		refuse "the base revision is not a full git commit id"
	fi
	printf 'first_release=false\nbase_version=%s\nbase_revision=%s\n' "$version" "$revision"
	;;
*)
	refuse "the record names no patch base"
	;;
esac
