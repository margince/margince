#!/usr/bin/env bash
# The license gate's coverage check: every package a committed pnpm-lock.yaml
# resolves must be an npm component of the CycloneDX SBOM that grant judges.
#
# grant judges only what the SBOM lists, and a scan that read part of a lockfile
# still writes a valid, non-empty document. pnpm writes the lockfile as a stream
# of YAML documents, its own pinned binaries first and the workspace after, so a
# reader that stops after the first one lists pnpm and none of the web app. That
# SBOM passes every format check and the license gate; only a count against the
# lockfile tells it apart.
#
# The lockfiles are read by PyYAML's safe_load_all, from the hash-pinned set that
# sbom-validate installs, in the pinned Python image: a reader that shares no code
# with the scanner it checks. A layout change can still blind both at once, so
# every document must declare the lockfile version both were checked against and
# resolve at least one package.
#
# The verdict goes to stderr, because sbom-gate discards the stdout of make sbom.
#
# Usage: sbom-census.sh <python-image> <requirements> <cyclonedx-sbom> [<lockfile>...]
# Without lockfiles it reads every pnpm-lock.yaml committed at HEAD, the tree
# `make sbom` scanned.
set -euo pipefail
export LC_ALL=C
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

image="$1"
requirements="$2"
sbom="$3"
shift 3

# Under the repository, because the container sees only its read-only mount.
mkdir -p .tmp
work="$(mktemp -d "$root/.tmp/sbom-census.XXXXXX")"
trap 'rm -rf "$work"' EXIT

names=()
if [[ $# -gt 0 ]]; then
	for lockfile in "$@"; do
		names+=("$lockfile")
		cat -- "$lockfile" >"$work/${#names[@]}.yaml"
	done
else
	while IFS= read -r path; do
		names+=("HEAD:$path")
		git show "HEAD:$path" >"$work/${#names[@]}.yaml"
	done < <(git ls-tree -r --name-only HEAD | grep -E '(^|/)pnpm-lock\.yaml$')
fi
if [[ ${#names[@]} -eq 0 ]]; then
	echo "FAIL: HEAD holds no pnpm-lock.yaml, so the census has nothing to hold the SBOM to" >&2
	exit 1
fi

# Arguments: a name for the messages and a path in the container, per lockfile.
# A `packages:` key is one resolved package, `name@version`; snapshots add only
# the peer variants of those keys.
read_packages='
import sys
import yaml

# The layout this reader and syft were checked against.
KNOWN_VERSION = "9.0"

pairs = sys.argv[1:]
for name, path in zip(pairs[0::2], pairs[1::2]):
    with open(path, encoding="utf-8") as stream:
        try:
            documents = list(yaml.safe_load_all(stream))
        except yaml.YAMLError as error:
            sys.exit(f"FAIL: could not read {name} as YAML: {error}")
    if not documents:
        sys.exit(f"FAIL: {name} holds no YAML document")
    for number, document in enumerate(documents, 1):
        where = f"document {number} of {name}"
        if not isinstance(document, dict):
            sys.exit(f"FAIL: {where} is not a YAML mapping")
        version = str(document.get("lockfileVersion"))
        if version != KNOWN_VERSION:
            sys.exit(f"FAIL: {where} has lockfileVersion {version}, and the census reads {KNOWN_VERSION}."
                     " Check that syft reads the new layout, then raise KNOWN_VERSION in scripts/sbom-census.sh")
        packages = document.get("packages")
        if not isinstance(packages, dict) or not packages:
            sys.exit(f"FAIL: {where} resolves no package, which is what a layout the census cannot read looks like")
        for key in packages:
            print(key)
'
pairs=()
for i in "${!names[@]}"; do
	pairs+=("${names[$i]}" "/src/${work#"$root"/}/$((i + 1)).yaml")
done
docker run --rm -v "$root":/src:ro -w /src --platform=linux/amd64 "$image" bash -c '
	bash scripts/install-sbom-python.sh "$1" >&2 \
		|| { echo "FAIL: could not install the pinned YAML reader" >&2; exit 1; }
	program="$2"
	shift 2
	python3 -c "$program" "$@"' census "$requirements" "$read_packages" "${pairs[@]}" >"$work/lockfile.keys"
sort -u "$work/lockfile.keys" >"$work/lockfile.set"
resolved=$(wc -l <"$work/lockfile.set" | tr -d ' ')
lockfiles="$(printf '%s, ' "${names[@]}")"
lockfiles="${lockfiles%, }"

jq -r '.components[]? | select((.purl // "") | startswith("pkg:npm/")) | "\(.name)@\(.version)"' "$sbom" \
	| sort -u >"$work/sbom.set"
comm -23 "$work/lockfile.set" "$work/sbom.set" >"$work/missing"
missing=$(wc -l <"$work/missing" | tr -d ' ')
if [[ "$missing" -ne 0 ]]; then
	echo "FAIL: the SBOM lists $((resolved - missing)) of the $resolved npm packages resolved by $lockfiles, so grant never judged these:" >&2
	sed 's/^/  /' "$work/missing" >&2
	exit 1
fi
echo "OK: the SBOM lists all $resolved npm packages resolved by $lockfiles" >&2
