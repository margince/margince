#!/usr/bin/env bash
# ci-image-context.sh <dir> — write the CI image's build context into <dir> and
# print the image's tag: the sha256 of every file in that context.
#
# The context holds exactly what the build reads: the Dockerfile, the recipe
# `make tools` runs, and the pnpm pin. So the tag changes when one of them does
# and on no other edit, and two runs that print one tag build one image.
# package.json is cut down to its `packageManager` field for that reason: its
# description is prose that no build reads.
set -euo pipefail
out="${1:?usage: ci-image-context.sh <empty directory>}"
root="$(cd "$(dirname "$0")/.." && pwd)"

mkdir -p "$out"
if [[ -n "$(ls -A "$out")" ]]; then
	echo "ci-image-context: $out is not empty, and a stray file would change the tag" >&2
	exit 1
fi

cp "$root/.github/ci-image/Dockerfile" "$out/Dockerfile"
"$root/scripts/gate-tools-recipe.sh" >"$out/tools.sh"

pin="$(grep -oE '"packageManager":[[:space:]]*"pnpm@[^"]+"' "$root/package.json" || true)"
if [[ -z "$pin" ]]; then
	echo "ci-image-context: package.json names no pnpm packageManager, so the image cannot pin pnpm" >&2
	exit 1
fi
printf '{%s}\n' "$pin" >"$out/package.json"

(cd "$out" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum) | sha256sum | cut -c1-64
