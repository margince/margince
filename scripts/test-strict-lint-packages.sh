#!/usr/bin/env bash
# test-strict-lint-packages.sh — prove strict-lint-packages.sh names every
# package a change touched and nothing `./...` from backend/ would not reach.
# A package it missed would leave new code unlinted with the pass still green,
# so the fixture changes one file of each kind the scope must keep or drop.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
script="$here/strict-lint-packages.sh"
# shellcheck source=scripts/lib-git-fixture.sh
source "$here/lib-git-fixture.sh"
repo="$(mktemp -d)"
trap 'rm -rf "$repo"' EXIT
cd "$repo"
git init -q -b main
git config user.email test@example.invalid
git config user.name test

put() { mkdir -p "$(dirname "$1")" && printf 'package p\n// %s\n' "$2" >"$1"; }
put backend/doc.go base
put backend/internal/kept/a.go base
put backend/internal/gone/b.go base
put backend/internal/quiet/c.go base
put backend/tools/gen/d.go base
printf 'module tools\n' >backend/tools/go.mod
git add -A && git commit -qm base
git switch -qc topic

put backend/doc.go changed
put backend/internal/kept/a.go changed
git rm -q backend/internal/gone/b.go
put backend/internal/fresh/e.go new
put backend/internal/kept/testdata/f.go new
put backend/internal/_scratch/g.go new
put backend/tools/gen/d.go changed
printf 'notes\n' >backend/internal/quiet/README.md
git add -A && git commit -qm committed
put backend/internal/staged/h.go staged
git add backend/internal/staged/h.go
put backend/internal/untracked/i.go untracked

want=$'.\n./internal/fresh\n./internal/kept\n./internal/staged\n./internal/untracked'
got="$("$script" main)"
if [[ "$got" != "$want" ]]; then
	printf 'FAIL: strict-lint-packages.sh named\n%s\nwant\n%s\n' "$got" "$want"
	exit 1
fi

git reset -q --hard && git clean -qfd && git switch -q main
if [[ -n "$("$script" main)" ]]; then
	echo "FAIL: strict-lint-packages.sh named packages on a branch that changed none"
	exit 1
fi
if "$script" no-such-ref >/dev/null 2>&1; then
	echo "FAIL: strict-lint-packages.sh passed with a base ref that does not resolve"
	exit 1
fi
echo "ok: strict-lint-packages.sh names exactly the changed packages ./... reaches"
