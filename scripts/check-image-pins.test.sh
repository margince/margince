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
# shellcheck source=scripts/lib-git-fixture.sh
source "$SCRIPT_DIR/lib-git-fixture.sh"

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

# --- a job container written as a scalar ------------------------------------
#
# `container: <image>` never reaches the `image:` scan, so it has its own. It
# admits the `ci-image` job output, read straight, through a lane's
# `ci-image` input, or behind a condition that falls back to ''. Every other
# way of naming a container must carry a digest. Each case below must name
# its own verdict, so a gate that crashed cannot pass it.

plant_workflow() {
  plant "FROM scratch"
  printf '%s\n' "$1" > "$TMP/repo/.github/workflows/w.yml"
  ( cd "$TMP/repo" && git add -A )
}

# reported_as <verdict> answers whether the gate failed with that verdict.
reported_as() {
  local out
  out="$( cd "$TMP/repo" && ./scripts/check-image-pins.sh 2>&1 )" && return 1
  grep -qF "$1" <<<"$out"
}

plant_workflow "    container: redis:7"
if ! reported_as "UNPINNED CONTAINER"; then
  fail "a tag-only scalar container passed; it is a pull like any service image"
fi

plant_workflow "    container: \${{ matrix.image }}"
if ! reported_as "UNPINNED CONTAINER"; then
  fail "a container read from a matrix passed; nothing holds a matrix value to a digest"
fi

plant_workflow "    container: \${{ github.event_name == 'pull_request' && needs.ci-image.outputs.pinned-image || 'redis:7' }}"
if ! reported_as "UNPINNED CONTAINER"; then
  fail "a fallback to a tag passed; only an empty fallback is admitted beside the CI image"
fi

plant_workflow "      ci-image: \${{ matrix.image }}"
if ! reported_as "CI IMAGE INPUT NOT FROM THE ci-image JOB"; then
  fail "a lane's ci-image input fed from somewhere other than the ci-image job passed"
fi

plant_workflow "on:
  workflow_call:
    inputs:
      ci-image:
        type: string
        default: redis:7
jobs:
  a:
    container: \${{ inputs.ci-image }}"
if ! reported_as "CI IMAGE INPUT DECLARED OUTSIDE workflow_call OR WITH A DEFAULT"; then
  fail "a ci-image input with a tag for its default passed; no caller is needed to reach container:"
fi

plant_workflow "on:
  workflow_dispatch:
    inputs:
      ci-image:
        type: string
jobs:
  a:
    container: \${{ inputs.ci-image }}"
if ! reported_as "CI IMAGE INPUT DECLARED OUTSIDE workflow_call OR WITH A DEFAULT"; then
  fail "a ci-image input a manual run can fill passed; no scan sees what is typed into a dispatch"
fi

plant_workflow "on:
  workflow_call:
    inputs:
      ci-image: # the caller's ci-image job output
        type: string
        default: ''
jobs:
  ci-image:
    runs-on: x
  b:
    container: redis:7@$DIGEST
    container: \${{ needs.ci-image.outputs.pinned-image }}
    container: \${{ inputs.ci-image }}
    container: \${{ matrix.where == 'image' && needs.ci-image.outputs.pinned-image || '' }}
    container: # the mapping form follows
      image: redis:7@$DIGEST
    with:
      ci-image: \${{ needs.ci-image.outputs.pinned-image }}"
if reported; then
  fail "a digest-pinned container, the CI image's output, its lane input or a mapping was reported"
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
