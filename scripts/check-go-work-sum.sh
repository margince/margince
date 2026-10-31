#!/usr/bin/env bash
# go.work.sum gate: the committed file is what `make install` leaves behind.
#
# `make install` runs `go mod download` in every module, and Go writes the
# workspace checksums it is missing into go.work.sum. A stale go.work.sum is
# therefore rewritten on a fresh checkout's first install, and a consumer that
# pins this repository as a submodule and requires it to stay unmodified fails
# there. Builds are unaffected, which is why nothing else notices.
#
# The download runs against a copy of go.work in a temporary directory (GOWORK),
# so this gate never writes to the working tree and can run beside the others.
# The module list is the one `make install` derives; keep the two in step.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cp go.work go.work.sum "$tmp/"
# The use paths are relative to go.work; the copy needs them absolute.
for use in $(go work edit -json | sed -n 's/.*"DiskPath": "\(.*\)".*/\1/p'); do
  go work edit -dropuse="$use" -use="$root/${use#./}" "$tmp/go.work"
done

for mod in $(git ls-files '*go.mod' | sed -e 's#/go\.mod$##' -e 's#^go\.mod$#.#' | grep -v '^fixtures/'); do
  (cd "$mod" && GOWORK="$tmp/go.work" go mod download)
done

if ! diff -u go.work.sum "$tmp/go.work.sum" >"$tmp/diff"; then
  echo "FAIL: go.work.sum is not what \`make install\` leaves behind:"
  sed -n '3,$p' "$tmp/diff" | grep '^[+-]' | sed 's/^/  /'
  echo
  echo "Run: make install, then commit go.work.sum"
  exit 1
fi

echo "OK: go-work-sum — go.work.sum is unchanged by \`make install\`"
