#!/usr/bin/env bash
# The image-pin gate's own test, for the half that was just added: Dockerfile
# base images.
#
# The `uses:` and `image:` halves have been in CI long enough to have been
# proven by use. The FROM half has not, and it is the shape most likely to fail
# SHORT: a scan that stopped recognising this tree's FROM lines finds nothing to
# object to and reports the same "image pins OK" as a fully pinned tree. So the
# cases below plant Dockerfiles in a throwaway repository — `git ls-files` is
# what the gate walks, so the fixture is a real one — and each says why it must
# be judged the way it is.
#
# Usage: bash scripts/check-image-pins.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GATE="$SCRIPT_DIR/check-image-pins.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

FAILURES=0
fail() { echo "FAIL: $*" >&2; FAILURES=$((FAILURES + 1)); }

readonly DIGEST="sha256:0000000000000000000000000000000000000000000000000000000000000000"

# plant builds a throwaway repository holding one Dockerfile and the gate.
#
# A real `git init`, because the gate reads `git ls-files`: a fixture the tree
# does not track is one the gate would skip, and a test that passed on skipped
# files would certify nothing.
plant() {
  local body="$1"
  rm -rf "$TMP/repo"
  mkdir -p "$TMP/repo/scripts" "$TMP/repo/.github/workflows"
  cp "$GATE" "$TMP/repo/scripts/check-image-pins.sh"
  printf '%s\n' "$body" > "$TMP/repo/Dockerfile"
  ( cd "$TMP/repo" && git init -q . && git add -A )
}

# reported runs the gate over the planted tree and answers whether it objected.
reported() {
  ! ( cd "$TMP/repo" && ./scripts/check-image-pins.sh >/dev/null 2>&1 )
}

# --- must be reported: an upstream image with no digest ---------------------

plant "FROM alpine:3.24 AS api"
if ! reported; then
  fail "a tag-only FROM passed — the case this half of the gate exists for"
fi

plant "FROM --platform=\$BUILDPLATFORM golang:1.26-alpine AS gobase"
if ! reported; then
  fail "a tag-only FROM behind a --platform flag passed; the flag is not the image"
fi

plant "FROM alpine@sha256:notadigest AS api"
if ! reported; then
  fail "a malformed digest passed — the pattern must bind 64 hex characters, not the word sha256"
fi

# The multi-stage shape this repository actually has: one pinned base, and a
# later stage that is NOT pinned. A gate reading only the first FROM would miss
# the second, which is the runtime image.
plant "FROM golang:1.26-alpine@$DIGEST AS build
FROM alpine:3.24 AS runtime"
if ! reported; then
  fail "an unpinned SECOND stage passed; every FROM is a pull, not just the first"
fi

# --- must NOT be reported ---------------------------------------------------

plant "FROM --platform=\$BUILDPLATFORM golang:1.26-alpine@$DIGEST AS gobase
FROM gobase AS api-build
FROM scratch AS empty
FROM alpine:3.24@$DIGEST AS api"
if reported; then
  fail "a fully pinned file was reported: a stage reference (FROM gobase) and FROM scratch pull nothing"
fi

# A stage name that also reads like an image name. The stage list is collected
# from the file itself, so this resolves as the stage it is rather than as an
# unpinned pull of something called `alpine`.
plant "FROM alpine:3.24@$DIGEST AS alpine
FROM alpine AS second"
if reported; then
  fail "a later stage named after an image was read as an unpinned pull"
fi

# --- and the gate must actually be reading this repository ------------------
#
# The cases above prove the rule; this proves the corpus. A gate whose file list
# came back empty would pass every one of them and say nothing about the tree it
# runs in.
tracked=$(cd "$SCRIPT_DIR/.." && git ls-files '*Dockerfile' '*Dockerfile.*' | wc -l | tr -d ' ')
if [[ "$tracked" -lt 1 ]]; then
  fail "the gate's own file list finds no Dockerfile in this repository — it would report OK over anything"
fi

if [[ "$FAILURES" -gt 0 ]]; then
  echo "image-pin gate: $FAILURES case(s) failed" >&2
  exit 1
fi
echo "==> image-pin verdicts: unpinned bases named, stage references and scratch pass ($tracked Dockerfile(s) in scope)"
