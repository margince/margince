#!/usr/bin/env bash
#
# home-budget-scope.sh — whether a change can make Home slower, so the advisory
# home-budget CI job skips its run when it cannot.
#
#   scripts/home-budget-scope.sh BASE_REF
#
# It runs when the diff against BASE_REF holds:
#   - any file under backend/ that is Go or a migration: Home's reads cross
#     most of the api, and no map from route to package is derived today;
#   - a shared dependency: go.mod, pnpm-lock.yaml, the Vite config, the
#     frontend's package.json or index.html;
#   - a file of the check itself;
#   - a frontend source file that Home's first load imports, by the import graph
#     (frontend/scripts/home-reach.ts). That last test needs node_modules.
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

changed="$(git -C "$ROOT" diff --name-only "$base"...HEAD)"
frontend=()
while IFS= read -r path; do
  case "$path" in
    "") ;;
    backend/*.go | backend/migrations/*)
      decide true "$path can change Home's reads" ;;
    go.mod | backend/go.mod | pnpm-lock.yaml | frontend/vite.config.ts | frontend/package.json | frontend/index.html)
      decide true "$path is a dependency Home shares" ;;
    scripts/home-budget*.sh | .github/workflows/ci.yml | frontend/e2e/bench-home.spec.ts | frontend/e2e/homejourney.ts | \
      frontend/src/testing/homejourney.judge.ts | frontend/scripts/home-reach.ts | frontend/scripts/lib/*)
      decide true "$path is part of the check" ;;
    frontend/src/*)
      frontend+=("$path") ;;
  esac
done <<<"$changed"

if ((${#frontend[@]} > 0)) && (cd "$ROOT/frontend" && node scripts/home-reach.ts "${frontend[@]}"); then
  decide true "Home's first load imports a changed frontend file"
fi
decide false "nothing in the diff can reach Home"
