#!/usr/bin/env bash
# The contract-fetch gate's own census — it tests the gate's SCOPE and its
# derivation, not its verdict on today's tree.
#
# Three ways this gate could report PASS while seeing nothing, and one case
# apiece:
#
#   1. It reads frontend/src and not extensions/*/frontend, so a unit screen
#      hand-rolls contract calls unread. Planted at both depths, because "the
#      tier is read" and "the tier is read to the bottom" are different claims
#      and the second is the one that broke in the design-system gates before.
#   2. It refuses a mount it has written down rather than the mount the contract
#      declares, so moving the mount silently empties the gate. Pointed at a
#      contract whose mount is /v2: the /v2 call has to fail it and the /v1 call
#      must not.
#   3. It honours any waiver comment, reason or no reason, so the exception
#      becomes free.
#
# The verdicts assert on the FIXTURE path, never on the exit code alone —
# frontend/src is scanned on the same run, so "the gate failed" would also be
# true when someone else's in-flight edit is what failed it.
#
# Usage: bash frontend/scripts/check-contract-fetch.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATE="$SCRIPT_DIR/check-contract-fetch.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0
fail() {
  echo "FAIL: $*" >&2
  FAILURES=$((FAILURES + 1))
}

DEPTHS=(probe/frontend/screen.tsx probe/frontend/views/panel.tsx)

plant() { # DIR LINE — one file per depth, each carrying LINE
  local dir="$1" line="$2" rel
  for rel in "${DEPTHS[@]}"; do
    mkdir -p "$dir/$(dirname "$rel")"
    printf '%s\n' "$line" >"$dir/$rel"
  done
}

plant_pair() { # DIR ABOVE BELOW — a two-line file per depth
  local dir="$1" above="$2" below="$3" rel
  for rel in "${DEPTHS[@]}"; do
    mkdir -p "$dir/$(dirname "$rel")"
    printf '%s\n%s\n' "$above" "$below" >"$dir/$rel"
  done
}

contract() { # PATH MOUNT — the smallest crm.yaml this gate reads
  cat >"$1" <<YAML
openapi: 3.1.0
servers:
  - url: https://crm.example.com$2
    description: The installation host
  - url: http://localhost:8080$2
    description: Local development
paths: {}
YAML
}

run() { # DIR CONTRACT → gate output in $OUT, verdict in $VERDICT
  OUT="$TMP/out"
  if MARGINCE_EXT_DIR="$1" MARGINCE_CONTRACT="$2" "$GATE" >"$OUT" 2>&1; then
    VERDICT=0
  else
    VERDICT=1
  fi
}

expect_named() { # DIR LABEL — the gate must fail and name every planted file
  local dir="$1" label="$2" rel
  if [[ "$VERDICT" -eq 0 ]]; then
    fail "$label: the gate passed"
    sed 's/^/      /' "$OUT" >&2
    return
  fi
  for rel in "${DEPTHS[@]}"; do
    grep -qF "$dir/$rel" "$OUT" && continue
    fail "$label: the gate failed without naming $dir/$rel — it is not reading that file"
    sed 's/^/      /' "$OUT" >&2
  done
}

expect_silent() { # DIR LABEL — the gate must not name any planted file
  local dir="$1" label="$2" rel
  for rel in "${DEPTHS[@]}"; do
    if grep -qF "$dir/$rel" "$OUT"; then
      fail "$label: the gate named $dir/$rel"
      sed 's/^/      /' "$OUT" >&2
    fi
  done
}

V1="$TMP/contract-v1.yaml"
V2="$TMP/contract-v2.yaml"
contract "$V1" /v1
contract "$V2" /v2

# 1. The extension tier is read, to the bottom.
DIRTY="$TMP/ext-dirty"
plant "$DIRTY" 'const r = await fetch("/v1/deals", { method: "GET" });'
run "$DIRTY" "$V1"
expect_named "$DIRTY" "a unit screen calling a contract path"

CLEAN="$TMP/ext-clean"
plant "$CLEAN" 'const r = await api.GET("/deals");'
run "$CLEAN" "$V1"
expect_silent "$CLEAN" "a unit screen using the generated client"

# 2. The mount comes from the contract. Same fixture, two contracts: which call
#    is refused has to follow the contract, not this gate.
run "$DIRTY" "$V2"
expect_silent "$DIRTY" "a /v1 call against a contract mounted at /v2"

V2_DIRTY="$TMP/ext-v2"
plant "$V2_DIRTY" 'const r = await fetch("/v2/deals", { method: "GET" });'
run "$V2_DIRTY" "$V2"
expect_named "$V2_DIRTY" "a /v2 call against a contract mounted at /v2"

run "$V2_DIRTY" "$V1"
expect_silent "$V2_DIRTY" "a /v2 call against a contract mounted at /v1"

# An absolute URL onto the installation host is the same call with a longer
# spelling, and a gate that only matched a leading slash would miss it.
ABS="$TMP/ext-abs"
plant "$ABS" 'const r = await fetch(`${origin}/v1/deals`);'
run "$ABS" "$V1"
expect_named "$ABS" "a contract path built onto the origin"

# A path assembled across adjacent literals is the same call again, and the
# separator lands in the SECOND literal — so a pattern demanding the mount be
# followed by a slash reads straight past it.
SPLIT_PATH="$TMP/ext-split-path"
plant "$SPLIT_PATH" 'const r = await fetch("/v1" + "/deals", { method: "GET" });'
run "$SPLIT_PATH" "$V1"
expect_named "$SPLIT_PATH" "a contract path split across two literals"

# The mount alone, with nothing after it: still a hand-written call to the
# contract's own root.
ROOT="$TMP/ext-root"
plant "$ROOT" 'const r = await fetch("/v1");'
run "$ROOT" "$V1"
expect_named "$ROOT" "a call to the mount root"

# And the split spelling still follows the CONTRACT: /v1 pieces are silent
# against a contract mounted at /v2, so the looser match did not become a
# hard-coded /v1.
run "$SPLIT_PATH" "$V2"
expect_silent "$SPLIT_PATH" "a split /v1 path against a contract mounted at /v2"

# 3. A waiver carries a reason or it is not a waiver.
WAIVED="$TMP/ext-waived"
plant_pair "$WAIVED" \
  '// contract-fetch:allow multipart — the client serializes JSON only' \
  'const r = await fetch("/v1/attachments", { method: "POST", body: form });'
run "$WAIVED" "$V1"
expect_silent "$WAIVED" "a waiver with a reason"

BARE="$TMP/ext-bare"
plant_pair "$BARE" \
  '// contract-fetch:allow' \
  'const r = await fetch("/v1/attachments", { method: "POST", body: form });'
run "$BARE" "$V1"
expect_named "$BARE" "a waiver with no reason"

# 4. A miswired scan fails closed rather than reporting PASS over nothing.
EMPTY="$TMP/ext-empty"
mkdir -p "$EMPTY"
if MARGINCE_EXT_DIR="$EMPTY" MARGINCE_CONTRACT="$TMP/absent.yaml" "$GATE" >"$TMP/out" 2>&1; then
  fail "the gate passed with no contract to derive a mount from"
  sed 's/^/      /' "$TMP/out" >&2
fi

SPLIT="$TMP/contract-split.yaml"
cat >"$SPLIT" <<'YAML'
openapi: 3.1.0
servers:
  - url: https://crm.example.com/v1
    description: The installation host
  - url: http://localhost:8080/v2
    description: Local development
paths: {}
YAML
if MARGINCE_EXT_DIR="$EMPTY" MARGINCE_CONTRACT="$SPLIT" "$GATE" >"$TMP/out" 2>&1; then
  fail "the gate passed with two contract mounts, so it picked one silently"
  sed 's/^/      /' "$TMP/out" >&2
fi

if [[ "$FAILURES" -ne 0 ]]; then
  echo "" >&2
  echo "The contract-fetch gate reads frontend/src AND extensions/*/frontend, takes" >&2
  echo "its mount from crm.yaml's servers block, and honours a waiver only when it" >&2
  echo "carries a reason. Each of those is one of the ways it could pass while" >&2
  echo "seeing nothing." >&2
  exit 1
fi

echo "==> contract-fetch scope: extension tier read at both depths, mount derived from the contract, waivers require a reason"
