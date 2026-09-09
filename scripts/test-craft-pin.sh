#!/usr/bin/env bash
# test-craft-pin.sh — prove the pinned gate is actually pinned.
#
# The failure this exists for is the quiet one: a resolver that fell back to
# PATH, or skipped verification, hands back *a* craft and every lane goes green
# against an unknown rubric. Nothing downstream can tell the difference. So the
# properties checked here are the ones that have no other witness.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fails=0

fail() {
	echo "FAIL: $*" >&2
	fails=$((fails + 1))
}

pinned() { # pinned <var-name> — read one pinned value out of the resolver
	local line
	line="$(grep "^${1}=" "$root/scripts/craft-pin.sh" || true)"
	line="${line#*=}"
	echo "${line//\"/}"
}

# 1. All four digests are pinned and none is a placeholder. A missing digest for
#    one platform is invisible to everybody not standing on it.
for platform in darwin_arm64 darwin_amd64 linux_arm64 linux_amd64; do
	digest="$(pinned "CRAFT_SHA256_${platform}")"
	if [[ ! "$digest" =~ ^[0-9a-f]{64}$ ]]; then
		fail "CRAFT_SHA256_${platform} is not a sha256: '${digest}'"
	fi
done

version="$(pinned CRAFT_VERSION)"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
	fail "CRAFT_VERSION is not a release tag: '${version}'"
fi

# 2. The resolver yields the gate, and the gate is the pinned one. A binary that
#    answers `version` with a different tuple is a different reviewer.
bin="$("$root/scripts/craft-pin.sh")"
if [[ ! -x "$bin" ]]; then
	fail "craft-pin.sh did not yield an executable: '$bin'"
elif ! "$bin" version >/dev/null 2>&1; then
	fail "the resolved binary does not answer 'craft version' — it is not the gate"
elif [[ "$bin" != *"/.tmp/craft/${version}/craft" ]]; then
	fail "resolved '$bin', which is not the cache path for ${version} — it came from somewhere else"
fi

# 3. The cached binary still matches its pinned digest. This is the property that
#    makes the pin worth having: without it the cache is a directory anyone can
#    drop a different gate into, and every lane would go green against it.
platform="$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/^x86_64$/amd64/;s/^aarch64$/arm64/')"
want="$(pinned "CRAFT_SHA256_${platform}")"
got="$(shasum -a 256 "$bin" | cut -d' ' -f1)"
if [[ "$want" != "$got" ]]; then
	fail "the cached binary does not match its pinned digest
  pinned: $want
  cached: $got
  Delete .tmp/craft/ and re-run; if it recurs, the release was changed."
fi

if ((fails > 0)); then
	echo "test-craft-pin: $fails failure(s)" >&2
	exit 1
fi
echo "OK: test-craft-pin (craft ${version}, digest verified)"
