#!/bin/sh
# Write the third-party notice file that ships beside the Go programs.
#
# The frontend notice file covers the SPA only. The three programs link Go
# modules, the Go toolchain's runtime and a few embedded binaries, and each of
# those carries a licence that asks for its text to travel with the binary.
#
# WHAT IT READS. The corpus is `go list -deps ./cmd/...`, the packages the
# three programs link, so a new dependency is covered the day it is added. A
# module whose directory is inside this repository is Margince's own code and
# is left out; LICENSE ships beside the programs. The embedded binaries come
# from embedded-binaries.txt, the declaration that
# scripts/embedded-binary-licenses.sh holds to the build.
#
# A linked module with no LICENSE, NOTICE or COPYING file in its root fails the
# run: a notice file that quietly skips a module claims a completeness it lacks.
#
# POSIX sh, not bash: the Go builder image that runs it has no bash.
#
# Usage: sh scripts/backend-notices.sh <output-file>
# Environment, for scripts/backend-notices.test.sh to plant a corpus:
#   BACKEND_NOTICES_MODULES  file of `path|version|dir` lines, replacing `go list`
#   BACKEND_NOTICES_GOROOT   directory holding the Go toolchain's LICENSE
#   BACKEND_NOTICES_DECLARED the embedded-binary declaration, default embedded-binaries.txt
set -eu

out="${1:-}"
if [ -z "$out" ]; then
	echo "backend-notices: pass the output file." >&2
	exit 1
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
declared="${BACKEND_NOTICES_DECLARED:-$root/embedded-binaries.txt}"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

if [ -n "${BACKEND_NOTICES_MODULES:-}" ]; then
	cp "$BACKEND_NOTICES_MODULES" "$work/all"
else
	(cd "$root/backend" && go list -deps -f '{{with .Module}}{{.Path}}|{{.Version}}|{{.Dir}}{{end}}' ./cmd/...) >"$work/all"
fi
goroot="${BACKEND_NOTICES_GOROOT:-$(go env GOROOT)}"

# Own code lives under the repository root; everything else is third party.
grep -v '^$' "$work/all" | sort -u | awk -F'|' -v root="$root/" 'index($3, root) != 1' >"$work/modules"
if [ ! -s "$work/modules" ]; then
	echo "backend-notices: the programs link no third-party module, so the corpus is wrong." >&2
	exit 1
fi

license_files() {
	find "$1" -maxdepth 1 -type f \( -iname 'LICEN[CS]E*' -o -iname 'NOTICE*' -o -iname 'COPYING*' -o -iname 'UNLICENSE*' \) | sort
}

section() {
	printf '\n================================================================================\n%s\n================================================================================\n' "$1"
}

{
	echo "Third-party notices for the Margince server programs"
	echo "(margince-api, margince-worker, margince-migrate)."
	echo
	echo "The programs link the Go toolchain's runtime, the Go modules below and the"
	echo "embedded binaries declared at the end. The licence text of each follows."
	echo "The web application's notices are in its own licenses/ folder."

	section "Go toolchain and standard library"
	[ -f "$goroot/LICENSE" ] || {
		echo "backend-notices: no LICENSE in the Go toolchain at $goroot." >&2
		exit 1
	}
	cat "$goroot/LICENSE"

	while IFS='|' read -r path version dir; do
		files="$(license_files "$dir")"
		if [ -z "$files" ]; then
			echo "backend-notices: $path $version has no LICENSE, NOTICE or COPYING file in $dir." >&2
			exit 1
		fi
		section "$path $version"
		for file in $files; do
			printf -- '--- %s ---\n' "$(basename "$file")"
			cat "$file"
			echo
		done
	done <"$work/modules"

	section "Embedded binaries"
	echo "Compiled files the programs carry, with what each was built from and that"
	echo "source's licence: module | file | built from | licence."
	echo
	grep -vE '^[[:space:]]*(#|$)' "$declared"
} >"$work/notices"

mkdir -p "$(dirname "$out")"
mv "$work/notices" "$out"
