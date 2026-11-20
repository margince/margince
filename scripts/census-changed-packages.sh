#!/usr/bin/env bash
# census-changed-packages.sh <base-ref> — prints, one import path per line, every
# package with tests whose test binary is built from a Go file that differs from
# the merge base with <base-ref> (tracked edits and untracked files alike).
# The packages are those whose binary depends on a changed package, since any
# production helper can start reading a file outside its module. Run in
# backend/ under the workspace the tests use.
set -euo pipefail

base="$(git merge-base "$1" HEAD)"
dirs="$( { git diff --relative --name-only "$base" -- '*.go'; git ls-files --others --exclude-standard -- '*.go'; } | xargs -n1 dirname 2>/dev/null | sort -u || true)"
existing=()
for dir in $dirs; do
	[ ! -d "$dir" ] || existing+=("./$dir")
done
[ "${#existing[@]}" -gt 0 ] || exit 0

changed="$(go list "${existing[@]}")"
go list -test -f '{{.ImportPath}} {{join .Deps " "}}' ./... ./tools/... |
	CHANGED="$changed" awk '
		BEGIN { n = split(ENVIRON["CHANGED"], list, "\n"); for (i = 1; i <= n; i++) hit[list[i]] = 1 }
		$1 ~ /\.test$/ {
			line = $0
			gsub(/ \[[^]]*\]/, "", line)
			count = split(line, field, " ")
			pkg = field[1]; sub(/\.test$/, "", pkg)
			for (i = 1; i <= count; i++) {
				if (i == 1 ? hit[pkg] : hit[field[i]]) { print pkg; break }
			}
		}' | sort -u
