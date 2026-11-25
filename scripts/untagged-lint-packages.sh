#!/usr/bin/env bash
# untagged-lint-packages.sh — prints the backend packages `make lint-untagged`
# lints, one `./dir` per line: each one holding a .go file that the untagged
# build compiles and the golangci build tags exclude.
#
# golangci builds with the tags in backend/.golangci.yml, so a file guarded by
# `//go:build !integration` is one no linter reads. It ships in every binary
# `go build ./...` makes. This list is the packages that hold such a file, so the
# untagged lint pass reads them without linting the whole tree twice.
#
# Only a constraint with a negation can admit a file without tags and drop it
# with more tags, so the grep below is a superset; `go list`, asked both ways,
# decides. backend/gates/lintbuildtagreach_test.go fails when this list misses
# a package or names one it should not.
set -euo pipefail
export LC_ALL=C

backend="$(cd "$(dirname "${BASH_SOURCE[0]}")/../backend" && pwd -P)"
cd "$backend"

# The `- tag` items under run.build-tags, comments skipped.
tags="$(awk '
	/^[[:space:]]*build-tags:/ { inside = 1; next }
	inside && /^[[:space:]]*#/ { next }
	inside && /^[[:space:]]*-[[:space:]]/ { sub(/^[[:space:]]*-[[:space:]]*/, ""); sub(/[[:space:]].*/, ""); printf "%s%s", sep, $0; sep = ","; next }
	inside { exit }
' .golangci.yml)"
if [[ -z "$tags" ]]; then
	echo "untagged-lint-packages: no run.build-tags in backend/.golangci.yml" >&2
	exit 1
fi

# backend/tools is a module of its own, which `./...` from here does not reach.
# backend/gates holds a hundred test-only files guarded `!integration`. They are
# the gate suite itself, and `make vet` reads them; linting that package a second
# time costs more than its findings would be worth.
candidates="$(grep -rlE --include='*.go' '^//(go:build|[[:space:]]*\+build).*!' . |
	grep -vE '^\./tools/|/(testdata|vendor)/|/[._][^/]*/' |
	sed 's#/[^/]*$##' | sort -u | grep -vx './gates' || true)"
[[ -n "$candidates" ]] || exit 0

files() {
	# shellcheck disable=SC2086 # one argument per package directory
	go list -e -tags="$1" -f '{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}{{range .TestGoFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}{{range .XTestGoFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}' $candidates | sort -u
}
comm -23 <(files "") <(files "$tags") | sed -e "s#^$backend/#./#" -e 's#/[^/]*$##' | sort -u
