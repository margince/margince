#!/usr/bin/env bash
# The release version this bake will stamp into every role, or a refusal.
#
# VERSION becomes three things from one variable: the image tag, the OCI version
# label, and the MARGINCE_RELEASE_VERSION build argument every role's binary is
# linked with. An empty or "dev" value builds and pushes perfectly well — and
# ships a set whose mixed-release guard is INERT on every installation that pulls
# it, because internal/shared/buildinfo.Comparable reads both as "this build does
# not know" and unknown disables every comparison.
#
# That is the correct fail-safe for a local build. It is the wrong outcome for a
# published release, and nothing downstream can tell the two apart: the images
# look finished, the roles all start, and the guard an operator believes is
# protecting them refuses nothing.
#
# So the one path that must always stamp proves it, here, before the bake.
# Checked in a script rather than trusted from the bake file, because the bake
# READS this variable — a bake that stayed correct while the value went missing
# would stamp "dev" and say nothing.
#
# Usage: scripts/release-version-stamped.sh "$VERSION"
set -euo pipefail

version="${1-}"

case "$version" in
"" | dev)
	echo "refusing to publish a release stamped '${version}'." >&2
	echo "  Every role reads that as 'this build carries no release version', so the" >&2
	echo "  published set would carry no mixed-release guard: each role would start" >&2
	echo "  against any other release rather than refusing a torn tag pull." >&2
	echo "  VERSION must be non-empty and must not be 'dev' — those two are what" >&2
	echo "  every role reads as 'no release version'. The release workflow supplies" >&2
	echo "  the drafted release here; any other stable identifier would pass this" >&2
	echo "  guard, because the roles compare for equality and never for order." >&2
	exit 1
	;;
esac

echo "$version"
