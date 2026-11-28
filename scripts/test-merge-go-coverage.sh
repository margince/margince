#!/usr/bin/env bash
# Tests for scripts/merge-go-coverage.sh: it merges real pods from the shards
# and the unit legs, and fails, naming the producer, when one sent nothing.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MERGE="$ROOT_DIR/scripts/merge-go-coverage.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }

# One real program, built with coverage, so every pod below is one go tool
# covdata reads. It names the backend's Go version, so the toolchain that
# builds it is the one the merge then runs.
mkdir -p "$work/prog"
printf 'module example.test/covered\n\n%s\n' "$(grep -m1 '^go ' "$ROOT_DIR/backend/go.mod")" > "$work/prog/go.mod"
cat > "$work/prog/main.go" <<'EOF'
package main

func covered() int { return 1 }

func main() { _ = covered() }
EOF
(cd "$work/prog" && GOWORK=off GOFLAGS= go build -cover -covermode=atomic -o "$work/prog/bin" .)
pod() { mkdir -p "$1" && GOCOVERDIR="$1" "$work/prog/bin"; }

# artifacts ROOT SHARDS LEGS... lays out what download-artifact leaves: a shard
# artifact with its meta.txt and pods, and a pod directory per unit leg.
artifacts() {
  local root="$1" shards="$2" k leg
  shift 2
  for k in $(seq 1 "$shards"); do
    mkdir -p "$root/integration-shard-$k-attempt-1"
    printf 'total=%s\n' "$shards" > "$root/integration-shard-$k-attempt-1/meta.txt"
    pod "$root/integration-shard-$k-attempt-1/covdata/part-1"
  done
  for leg in "$@"; do
    pod "$root/unit-covdata-$leg-attempt-1"
  done
}

# expect_fail NAME WANT ROOT: the merge refuses, and says WANT.
expect_fail() {
  local name="$1" want="$2" root="$3" out
  if out="$("$MERGE" "$root" "$work/$name.out" 2>&1)"; then
    fail "$name: the merge passed, want it to refuse"
  fi
  [[ "$out" == *"$want"* ]] || fail "$name: the merge said '$out', want it to name '$want'"
}

# 1. Every producer sent its pods: one profile, holding the program's block.
artifacts "$work/whole" 2 unit solo
"$MERGE" "$work/whole" "$work/whole.out" > "$work/whole.log" || fail "a complete set did not merge: $(cat "$work/whole.log")"
head -1 "$work/whole.out" | grep -qx 'mode: atomic' || fail "the profile does not start with its mode line"
grep -q 'example.test/covered/main.go' "$work/whole.out" || fail "the profile holds no block of the covered program"
grep -q '4 pod dirs (2 from 2 shards' "$work/whole.log" || fail "the merge did not count every pod: $(cat "$work/whole.log")"
grep -qE '^coverage: [0-9]+ of [0-9]+ statements covered' "$work/whole.log" || fail "the merge printed no covered share: $(cat "$work/whole.log")"

# 2. No shard artifact at all.
mkdir -p "$work/none"
pod "$work/none/unit-covdata-unit-attempt-1"
expect_fail none "no shard artifact was downloaded" "$work/none"

# 3. One shard of two sent nothing.
artifacts "$work/short" 2 unit solo
rm -rf "$work/short/integration-shard-2-attempt-1"
expect_fail short "shard 2/2 sent no artifact" "$work/short"

# 4. A unit leg sent nothing: its packages would read 0% to the scanner.
artifacts "$work/noleg" 1 unit
expect_fail noleg "unit leg 'solo'" "$work/noleg"

echo "OK: the merge holds every shard's and unit leg's pods, and refuses a set missing one"
