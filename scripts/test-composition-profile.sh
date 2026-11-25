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
for goal in build composition e2e-agent-loop check check-backend gen drift test-extensions repeat-changed-tests fe-quality migrate migrate-down; do
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
mkdir -p "$scratch/bin"
cat > "$scratch/bin/bash" <<'EOF_BASH'
#!/bin/bash
set -euo pipefail
if [[ "${1:-}" == */scripts/test-integration-one.sh ]]; then
  printf '%s\n' "${3:-}"
else
  exec /bin/bash "$@"
fi
EOF_BASH
chmod +x "$scratch/bin/bash"
for entry in "$root" "$root/backend"; do
  for pattern in '^One$' '^(One|OneMore)$' "^(One'Quoted|Two \"words\")$"; do
    actual="$(PATH="$scratch/bin:$PATH" env -u MAKEFLAGS -u MAKEOVERRIDES make --no-print-directory -s -C "$entry" test-it CHECK_WRITERS_RAN=1 DIR=backend/internal/compose RUN="$pattern" | tail -n 1)"
    if [[ "$actual" != "$pattern" ]]; then
      echo "FAIL: $entry forwarded test filter '$actual', want '$pattern'" >&2
      exit 1
    fi
  done
done
echo 'OK: test-it forwards literal filters through both Makefiles'
