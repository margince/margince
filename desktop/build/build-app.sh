#!/usr/bin/env bash
# Build the Margince-authored half of the bundle: the three process-role
# binaries, the frontend, and the launcher that supervises them.
#
# The server binaries are built through build/composition/, not with a bare
# `go build`, because that wiring is what links the enabled extensions/ units
# in. A bundle built against the vanilla stub would silently ship without the
# first-party packs and look identical from the outside.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Pins the deployment target before any compiler runs, so every binary here is
# stamped with the bundle's declared floor instead of the build machine's OS.
#
# Go needs this as much as the C halves do, and far less obviously. With no cgo
# anywhere in the graph Go links internally and stamps its own supported floor,
# which is the same number this file declares — so omitting it produced a
# correct bundle by coincidence rather than by decision. The coincidence ended
# when one dependency arrived carrying a darwin-only cgo file: that hands the
# link to clang, clang defaults the target to the builder's OS, and every
# binary in the bundle then demanded the macOS the runner happened to be on.
# shellcheck source=desktop/build/macos-target.sh
. "$HERE/macos-target.sh"
ROOT="$(cd "$HERE/../.." && pwd)"
OUT="$ROOT/build/desktop/.stage"

log() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }

build_server_binaries() {
  log "materializing build/composition"
  (cd "$ROOT/backend" && GOWORK="$ROOT/go.work" go run ./tools/gen-composition)

  local composition="$ROOT/build/composition/go.work"
  if [[ ! -f "$composition" ]]; then
    echo "FAIL: gen-composition did not produce $composition" >&2
    exit 1
  fi

  mkdir -p "$OUT/bin"
  local role
  for role in api worker migrate; do
    log "building $role"
    (cd "$ROOT/backend" && GOWORK="$composition" go build -o "$OUT/bin/$role" "./cmd/$role")
    sign_binary "$OUT/bin/$role"
  done
}

# The signed binaries, in build order. Read by verify_runnable_os below.
BUILT=()

# sign_binary ad-hoc signs one executable, HERE in the staging directory
# rather than after assembly.
#
# codesign treats a directory containing a same-named executable as a tool
# bundle, so signing the launcher once it sits in the distributable folder
# makes codesign try to sign that whole folder — and fail on the .command
# starter script it cannot sign as a subcomponent. Staging paths cannot
# collide that way. Signatures are embedded in the Mach-O, so they survive
# the copy into the folder.
#
# It also records what it signed, in BUILT. Every executable this script ships
# is signed, so this is the one place that already knows the whole list —
# assembling a second one, by globbing the staging directory or by naming the
# roles again, is how a census comes to check fewer binaries than were built.
sign_binary() {
  codesign --force --sign - --timestamp=none "$1"
  BUILT+=("$1")
}

# build_frontend builds the COMPOSED SPA, for the same reason the server
# binaries are built through build/composition/: a bare `pnpm build` resolves
# the committed empty-tree registry, so the bundle would ship a server with the
# enabled units linked and a UI that routes none of them. The Dockerfile's web
# stage is the reference for this lane.
build_frontend() {
  log "building the frontend (composed)"
  local registry="$ROOT/build/composition/frontend"
  if [[ ! -f "$registry/extensions.gen.ts" ]]; then
    echo "FAIL: gen-composition did not produce $registry/extensions.gen.ts" >&2
    exit 1
  fi

  # The published-event types are the one generated artifact whose composed
  # form is written back into the tracked source tree (frontend/src/api/), so
  # this lane refuses to run it: a throwaway Docker layer can afford a dirty
  # checkout and a developer's cannot. As long as no enabled unit contributes
  # a public event the two documents are identical and there is nothing to
  # generate; the moment one does, this says so instead of shipping types that
  # silently omit it.
  if ! diff -q "$ROOT/backend/api/public-events.yaml" "$ROOT/build/composition/api/public-events.yaml" >/dev/null; then
    echo "FAIL: an enabled extension contributes public events, so frontend/src/api/public-events.ts must be regenerated (pnpm gen:events:composed) before this bundle can ship correct types" >&2
    exit 1
  fi

  # Install at the REPO ROOT, build in frontend/. The lockfile is a root pnpm
  # workspace, so there is no frontend/pnpm-lock.yaml to install against, and
  # --frozen-lockfile run from frontend/ would either resolve the root lockfile
  # from a subdirectory or rewrite it. The Dockerfile's web stage splits the two
  # steps for the same reason and is the reference for this lane.
  #
  # --ignore-scripts matches every other frontend install in this repo (the
  # Dockerfile, scripts/verify-boot.sh, the CI lane): a lockfile pins WHAT is
  # installed, and this stops a dependency's lifecycle script from running
  # arbitrary code on the build machine on the way in.
  (cd "$ROOT" && pnpm install --frozen-lockfile --ignore-scripts)
  # THEN the composed workspace, and the order is load-bearing — the same order
  # `make fe-typecheck-composed` documents.
  #
  # A unit's frontend layer is NOT a member of the root workspace
  # (pnpm-workspace.yaml says why: membership would make an installation that
  # enables its own frontend-bearing unit unable to run --frozen-lockfile
  # against an upstream-owned lockfile). Its react, @tanstack/react-query and
  # @types/react are linked out of frontend/node_modules by the GENERATED
  # workspace instead, so the root install alone leaves a unit screen resolving
  # neither its peers nor its dev deps — and the composed build below
  # typechecks those screens.
  #
  # This lane got away without it while the root lockfile still carried stale
  # `extensions/*/frontend` entries from the era when they WERE members: they
  # supplied the peers by accident. A lockfile regeneration correctly dropped
  # them, and this build broke on the next change to touch desktop/ — the
  # failure being TS2307 "cannot find module 'react'" in a unit's screen, which
  # names neither this script nor the missing step.
  #
  # --no-frozen-lockfile because that lockfile is generated build output, which
  # is what the Makefile's own invocation says at greater length.
  local composed_ws="$ROOT/build/composition-frontend/workspace"
  if [[ ! -f "$composed_ws/pnpm-workspace.yaml" ]]; then
    echo "FAIL: gen-composition did not produce $composed_ws/pnpm-workspace.yaml, so a unit's frontend dependencies cannot be resolved" >&2
    exit 1
  fi
  (cd "$composed_ws" && pnpm install --no-frozen-lockfile --ignore-scripts)
  (cd "$ROOT/frontend" &&
    MARGINCE_COMPOSITION_FRONTEND="$registry" pnpm gen:composed-types &&
    MARGINCE_COMPOSITION_FRONTEND="$registry" pnpm build:composed)
  rm -rf "$OUT/web"
  cp -R "$ROOT/frontend/dist" "$OUT/web"
}

build_launcher() {
  # GOWORK=off: the launcher is a standalone stdlib-only module deliberately
  # outside the workspace, so it neither sees nor perturbs the backend's
  # dependency graph.
  log "building the launcher"
  (cd "$ROOT/desktop/launcher" && GOWORK=off go build -o "$OUT/bin/margince" .)
  sign_binary "$OUT/bin/margince"
}

# verify_runnable_os holds THIS step's output to the bundle's floor, the way the
# Postgres and bus steps hold theirs.
#
# build-dist.sh checks the assembled folder too, and that check is the one that
# constrains what a user copies. This one names the step that stamped a binary
# wrong while the build log still shows which compiler ran, and main() calls it
# before the frontend so the answer does not wait on a build that cannot change
# it.
verify_runnable_os() {
  # An empty census would pass while examining nothing, which is how a
  # verification step most often fails.
  if [[ ${#BUILT[@]} -eq 0 ]]; then
    echo "FAIL: no binaries were built, so there is nothing to hold to the macOS floor" >&2
    exit 1
  fi
  if ! assert_min_os "${BUILT[@]}"; then
    exit 1
  fi
  log "every binary runs on macOS $MACOS_MIN or newer"
}

main() {
  # Every binary, then the floor, then the frontend. The frontend is the
  # expensive half — a dependency install and two vite builds — and it produces
  # no Mach-O, so nothing in it can affect the answer. Paying for it before
  # asking is how the earlier order reported a stamped binary several minutes
  # after it could have.
  build_server_binaries
  build_launcher
  verify_runnable_os
  if [[ "${SKIP_FRONTEND:-0}" != "1" ]]; then
    build_frontend
  fi
  log "binaries in $OUT/bin"
}

main "$@"
