#!/usr/bin/env bash
#
# home-budget-scope.sh — whether a change can make Home slower, so the advisory
# home-budget CI job skips its run when it cannot.
#
#   scripts/home-budget-scope.sh BASE_REF
#   scripts/home-budget-scope.sh - < paths     (the changed paths, one per line)
#
# It runs when the diff against BASE_REF holds:
#   - any file under backend/ that is Go or a migration: Home's reads cross
#     most of the api, and no map from route to package is derived today;
#   - a shared dependency: go.mod, pnpm-lock.yaml, the Vite config, the
#     frontend's package.json or index.html;
#   - a file the check runs that no import reaches: its scripts and what they
#     call, the Makefiles, the Playwright config, the seed, the CI job;
#   - a frontend file that Home's first load or the browser check imports, by
#     the import graph (frontend/scripts/home-reach.ts). That needs node_modules.
#
# Prints the verdict and its reason. With GITHUB_OUTPUT set it also writes
# run=true or run=false there.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
base="${1:?usage: $0 BASE_REF}"

decide() {
  echo "home-budget-scope: run=$1 ($2)"
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "run=$1" >>"$GITHUB_OUTPUT"
  fi
  exit 0
}

if [[ "$base" == "-" ]]; then
  changed="$(cat)"
else
  changed="$(git -C "$ROOT" diff --name-only "$base"...HEAD)"
fi
frontend=()
while IFS= read -r path; do
  case "$path" in
    "") ;;
    backend/*.go | backend/migrations/*)
      decide true "$path can change Home's reads" ;;
    go.mod | backend/go.mod | pnpm-lock.yaml | frontend/vite.config.ts | frontend/package.json | frontend/index.html)
      decide true "$path is a dependency Home shares" ;;
    scripts/home-budget*.sh | scripts/lib-testdb.sh | scripts/seed-dev.sh | scripts/seed-dev.sql | scripts/lib-devstate.sh | \
      Makefile | backend/Makefile | docker-compose.dev.yml | .github/workflows/ci.yml | .github/actions/go-build-cache/* | .github/actions/install-playwright-chromium/* | \
      frontend/playwright.config.ts | frontend/scripts/home-reach.ts | frontend/scripts/lib/* | frontend/scripts/build-mcp-apps.mjs)
      decide true "$path is part of the check" ;;
    frontend/*)
      frontend+=("$path") ;;
  esac
done <<<"$changed"

if ((${#frontend[@]} > 0)) && (cd "$ROOT/frontend" && node scripts/home-reach.ts "${frontend[@]}"); then
  decide true "Home's first load or its check imports a changed frontend file"
fi
decide false "nothing in the diff can reach Home"
