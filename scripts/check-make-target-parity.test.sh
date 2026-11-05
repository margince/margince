#!/usr/bin/env bash
# The make-target-parity gate's own test. It copies the REAL root and backend
# Makefiles into a throwaway tree, so what passes is the forwarding rule this
# repository ships, then plants each mistake the gate exists for and requires
# the gate to fail on it. A gate that only ever reports OK cannot be told from
# one that stopped looking.
#
# Usage: bash scripts/check-make-target-parity.test.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$SCRIPT_DIR/.." && pwd)"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# plant builds one case's fixture in its own folder and applies a sed edit to
# its root Makefile (an empty edit keeps the shipped one). The tools/ and
# composition/ folders are the root names that collide with a backend target,
# which is what the forwarding rule's FORCE is for.
plant() {
  local repo="$TMP/$1" edit="$2"
  mkdir -p "$repo/scripts" "$repo/backend" "$repo/tools" "$repo/composition"
  cp "$REPO/Makefile" "$repo/Makefile"
  cp "$REPO/backend/Makefile" "$repo/backend/Makefile"
  cp "$SCRIPT_DIR/check-make-target-parity.sh" "$SCRIPT_DIR/make-help.sh" "$repo/scripts/"
  if [[ -n "$edit" ]]; then
    sed -i.orig -e "$edit" "$repo/Makefile"
    if cmp -s "$repo/Makefile" "$repo/Makefile.orig"; then
      echo "FAIL: case $1: the edit '$edit' changed nothing; the root Makefile no longer has the line it plants a defect in" >&2
      return 1
    fi
  fi
}

# judge runs the gate on one case's fixture and checks its verdict and message.
# Each dry-runs every backend target, so the cases run side by side.
judge() {
  local repo="$TMP/$1" want="$2" needle="$3" why="$4" out status=0
  out="$(bash "$repo/scripts/check-make-target-parity.sh" 2>&1)" || status=$?
  if [[ "$want" == pass && "$status" -ne 0 ]]; then
    echo "FAIL: $why: the gate failed on a sound tree:"$'\n'"$out" >&2
    return 1
  elif [[ "$want" == fail && "$status" -eq 0 ]]; then
    echo "FAIL: $why: the gate passed" >&2
    return 1
  elif ! grep -qF -- "$needle" <<<"$out"; then
    echo "FAIL: $why: the gate did not say '$needle':"$'\n'"$out" >&2
    return 1
  fi
}

plant shipped ""
plant phony 's/^\.PHONY: help help-all/.PHONY: vuln help help-all/'
plant noforce 's/^%: FORCE$/%:/'
plant swallow 's/^\t\$(MAKE) -C backend \$@$/\t-$(MAKE) -C backend $@/'
plant helpentry ""
sed -i.orig 's/^  "Release|/  "Release|no-such-help-entry /' "$TMP/helpentry/scripts/make-help.sh"

pids=()
judge shipped pass "make target parity OK" "the shipped Makefiles" & pids+=($!)
judge phony fail "SILENT FROM ROOT: \`make vuln\`" "a forwarded name in the root .PHONY list" & pids+=($!)
judge noforce fail "SILENT FROM ROOT: \`make tools\`" "a forwarding rule without FORCE, beside a tools/ folder" & pids+=($!)
judge swallow fail "SWALLOWED TYPO" "a forwarding rule that ignores backend's failure" & pids+=($!)
judge helpentry fail "\`no-such-help-entry\` is listed here" "a help entry no Makefile describes" & pids+=($!)

failures=0
for pid in "${pids[@]}"; do
  wait "$pid" || failures=$((failures + 1))
done
if [[ "$failures" -ne 0 ]]; then
  echo "test-make-target-parity: $failures case(s) failed" >&2
  exit 1
fi
echo "test-make-target-parity OK (the shipped Makefiles pass; four planted defects fail)"
