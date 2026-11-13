#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
cat > "$scratch/Makefile" <<EOF_MAKE
include $root/scripts/composition-profile.mk
print:
	@printf '%s|%s\n' '\$(COMPOSITION_FLAGS)' "\$\$COMPOSITION_FLAGS"
EOF_MAKE
for goal in build composition check check-backend gen drift test-extensions repeat-changed-tests fe-quality migrate migrate-down; do
  case "$goal" in build|composition) want='|' ;; *) want='-include-development|-include-development' ;; esac
  actual="$(env -u COMPOSITION_FLAGS -u MAKEFLAGS -u MAKEOVERRIDES make --no-print-directory -s -f "$scratch/Makefile" MAKECMDGOALS="$goal" print)"
  if [[ "$actual" != "$want" ]]; then
    echo "FAIL: $goal composition flags '$actual', want '$want'" >&2
    exit 1
  fi
done
actual="$(env -u MAKEFLAGS -u MAKEOVERRIDES make --no-print-directory -s -f "$scratch/Makefile" MAKECMDGOALS=check COMPOSITION_FLAGS= print)"
[[ "$actual" == '|' ]] || { echo 'FAIL: explicit production override was ignored' >&2; exit 1; }
cd "$root"
for entry in "$root" "$root/backend"; do
  env -u COMPOSITION_FLAGS -u MAKEFLAGS -u MAKEOVERRIDES make --no-print-directory -n -C "$entry" composition > "$scratch/production"
  env -u COMPOSITION_FLAGS -u MAKEFLAGS -u MAKEOVERRIDES make --no-print-directory -n -C "$entry" gen > "$scratch/development"
  if grep -E 'gen-composition.*-include-development' "$scratch/production" >/dev/null; then
    echo "FAIL: $entry production composition includes examples" >&2; exit 1
  fi
  grep -E 'gen-composition.*-include-development' "$scratch/development" >/dev/null || { echo "FAIL: $entry generation skips examples" >&2; exit 1; }
done
echo 'OK: composition profile defaults, export and override'
