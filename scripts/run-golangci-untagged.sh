#!/usr/bin/env bash
# run-golangci-untagged.sh — `make lint-untagged`: lint, with no build tags, the
# packages scripts/untagged-lint-packages.sh names. Run from backend/.
#
# golangci appends --build-tags to the config's run.build-tags rather than
# replacing it, so the untagged build needs a config without that list. It is
# derived from .golangci.yml on every run, so it cannot drift from the baseline,
# and it reads paths from backend/ as the original does from beside itself.
# Findings print with absolute paths: run-golangci.sh reads a relative one
# against the config's directory, which here is a temporary one.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
pkgs="$("$here/untagged-lint-packages.sh")"
if [[ -z "$pkgs" ]]; then
	echo "SKIP untagged lint: no package holds a file only the untagged build compiles"
	exit 0
fi

# golangci picks the parser by extension, so the file is named .yml.
scratch="$(mktemp -d "${TMPDIR:-/tmp}/golangci-untagged.XXXXXX")"
config="$scratch/golangci.yml"
trap 'rm -f "$config"; rmdir "$scratch"' EXIT
awk '
	/^run:/ { print; print "  relative-path-mode: wd"; next }
	/^  build-tags:/ { dropping = 1; next }
	dropping && /^(    |[[:space:]]*$)/ { next }
	{ dropping = 0; print }
' .golangci.yml >"$config"
if grep -q 'build-tags' "$config"; then
	echo "run-golangci-untagged: could not drop run.build-tags from .golangci.yml" >&2
	exit 1
fi

echo "lint-untagged: $(echo "$pkgs" | wc -l | tr -d ' ') package(s)"
# shellcheck disable=SC2086 # one argument per package
GOLANGCI_CACHE_PASS=untagged "$here/run-golangci.sh" run --config "$config" --path-mode abs $pkgs
