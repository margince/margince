#!/usr/bin/env bash
# lib-composedunits.sh's own test: which transports a composed unit list yields,
# and what it refuses.
#
# Usage: bash scripts/lib-composedunits.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/lib-composedunits.sh
. "$SCRIPT_DIR/lib-composedunits.sh"

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# plant REPO LIST — a tree whose units.json is LIST, with two unit manifests:
# one declaring a channel and one declaring none.
plant() {
  mkdir -p "$1/build/composition" "$1/extensions/chat" "$1/extensions/plain" "$1/extensions/demo"
  printf '%s\n' "$2" >"$1/build/composition/units.json"
  echo '{"name":"chat","channels":[{"provider":"chatwire"}]}' >"$1/extensions/chat/manifest.generated.json"
  echo '{"name":"plain"}' >"$1/extensions/plain/manifest.generated.json"
  echo '{"name":"demo","channels":[{"provider":"demowire"}]}' >"$1/extensions/demo/manifest.generated.json"
}

plant "$work/listed" '[{"name":"chat","manifest":"extensions/chat/manifest.generated.json"},{"name":"plain","manifest":"extensions/plain/manifest.generated.json"}]'
got="$(composed_unit_transports "$work/listed")"
[[ "$got" == "chatwire" ]] || fail "listed units: got '$got', want only chatwire (demo is on disk but not composed)"

plant "$work/none" '[{"name":"plain","manifest":"extensions/plain/manifest.generated.json"}]'
got="$(composed_unit_transports "$work/none")"
[[ -z "$got" ]] || fail "a unit with no channel: got '$got', want nothing"

plant "$work/core" '[]'
got="$(composed_unit_transports "$work/core")"
[[ -z "$got" ]] || fail "a core-only build: got '$got', want nothing"

plant "$work/gone" '[{"name":"lost","manifest":"extensions/lost/manifest.generated.json"}]'
if composed_unit_transports "$work/gone" >/dev/null 2>"$work/gone.err"; then
  fail "a listed manifest that is not there passed"
fi
grep -q "extensions/lost/manifest.generated.json" "$work/gone.err" || fail "the missing manifest is not named: $(cat "$work/gone.err")"

mkdir -p "$work/unlisted"
if composed_unit_transports "$work/unlisted" >/dev/null 2>&1; then
  fail "a tree with no units.json passed"
fi

if ((FAILURES > 0)); then
  echo "lib-composedunits.test.sh: $FAILURES failure(s)" >&2
  exit 1
fi
echo "lib-composedunits.test.sh: OK"
