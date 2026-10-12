#!/usr/bin/env bash
# The license gate's coverage check: every package pnpm-lock.yaml resolves must
# be an npm component of the CycloneDX SBOM that grant judges.
#
# grant judges only what the SBOM lists, and a scan that read part of the
# lockfile still writes a valid, non-empty document. pnpm writes the lockfile as
# a stream of YAML documents, its own pinned binaries first and the workspace
# after, so a reader that stops after the first one lists pnpm and none of the
# web app. That SBOM passes every format check and the license gate; only a
# count against the lockfile tells it apart.
#
# The lockfile is read by PyYAML's safe_load_all, from the hash-pinned set that
# sbom-validate installs, in the pinned Python image: a reader that shares no
# code with the scanner it checks.
#
# Usage: sbom-census.sh <python-image> <requirements> <cyclonedx-sbom> [<lockfile>]
# Without a lockfile it reads HEAD's, the tree `make sbom` scanned.
set -euo pipefail
export LC_ALL=C
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

image="$1"
requirements="$2"
sbom="$3"
lockfile="${4:-}"
lockfile_name="${lockfile:-HEAD:pnpm-lock.yaml}"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# A `packages:` key is one resolved package, `name@version`. Snapshots add only
# the peer variants of those keys, so they are not counted.
read_packages='
import sys
import yaml

for document in yaml.safe_load_all(sys.stdin):
    for key in (document or {}).get("packages") or {}:
        print(key)
'

if [[ -n "$lockfile" ]]; then
	cat -- "$lockfile" >"$work/lockfile.yaml"
else
	git show HEAD:pnpm-lock.yaml >"$work/lockfile.yaml"
fi
if ! docker run --rm -i -v "$root":/src:ro -w /src --platform=linux/amd64 "$image" bash -c '
	pip install --require-hashes --quiet --no-cache-dir --disable-pip-version-check --root-user-action=ignore -r "$1" >&2 \
		|| { echo "FAIL: could not install the pinned YAML reader" >&2; exit 1; }
	python3 -c "$2"' census "$requirements" "$read_packages" <"$work/lockfile.yaml" >"$work/lockfile.keys"; then
	echo "FAIL: could not read $lockfile_name with the pinned YAML reader" >&2
	exit 1
fi
sort -u "$work/lockfile.keys" >"$work/lockfile.set"
resolved=$(wc -l <"$work/lockfile.set" | tr -d ' ')

# Nothing resolved is what a lockfile layout this reader cannot see looks like,
# and against an empty census every SBOM would be complete.
if [[ "$resolved" -eq 0 ]]; then
	echo "FAIL: $lockfile_name resolves no package, so the census has nothing to hold the SBOM to" >&2
	exit 1
fi

jq -r '.components[]? | select((.purl // "") | startswith("pkg:npm/")) | "\(.name)@\(.version)"' "$sbom" \
	| sort -u >"$work/sbom.set"
comm -23 "$work/lockfile.set" "$work/sbom.set" >"$work/missing"
missing=$(wc -l <"$work/missing" | tr -d ' ')
if [[ "$missing" -ne 0 ]]; then
	echo "FAIL: the SBOM lists $((resolved - missing)) of the $resolved npm packages $lockfile_name resolves, so grant never judged these:" >&2
	sed 's/^/  /' "$work/missing" >&2
	exit 1
fi
echo "OK: the SBOM lists all $resolved npm packages $lockfile_name resolves"
