#!/usr/bin/env bash
# check-make-target-parity.sh — every backend target `make help-all` advertises
# runs from the repo root, a typo still fails there, and `make help` prints.
#
# The root Makefile forwards every name it does not define to backend/ through
# one pattern rule, so a new backend target needs no edit to be reachable. Two
# mistakes still break that without an error:
#
#   - a forwarded name added to the root .PHONY list. make looks for no pattern
#     rule for a phony target, so `make vuln` would print "Nothing to be done"
#     and exit 0, and a CI step calling it would pass without scanning;
#   - a forwarding rule that swallows unknown names, so `make chekc` succeeds.
#
# Each target is dry-run (`make -n`), which prints what it would run without
# running it; a dry run that prints nothing to do is the first mistake.
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
backend_makefile="$root_dir/backend/Makefile"

if [[ ! -f "$backend_makefile" ]]; then
  echo "check-make-target-parity: $backend_makefile not found" >&2
  exit 1
fi

# A sub-make of `make check` inherits the caller's flags and job server; this
# check asks make its own questions and must not inherit them.
root_make() {
  (cd "$root_dir" && env -u MAKEFLAGS -u MFLAGS -u MAKELEVEL make --no-print-directory "$@")
}

# The backend's advertised surface, spelled the way `make -C backend help`
# spells it: a target whose rule line carries a `## ` description.
advertised="$(grep -hE '^[a-zA-Z][a-zA-Z0-9_-]*:.*## ' "$backend_makefile" | cut -d: -f1 | sort -u)"

# An empty list would pass this gate while checking nothing.
if [[ -z "$advertised" ]]; then
  echo "check-make-target-parity: extracted no advertised backend targets — backend/Makefile changed shape, so this gate was about to pass without checking anything" >&2
  exit 1
fi

failed=0
while IFS= read -r target; do
  if ! dry_run="$(root_make -n "$target" 2>&1)"; then
    echo "UNREACHABLE FROM ROOT: \`make $target\` is advertised by \`make help-all\` but fails from the repo root:" >&2
    printf '%s\n' "$dry_run" | tail -3 >&2
    failed=1
  elif grep -qE 'Nothing to be done|is up to date' <<<"$dry_run"; then
    echo "SILENT FROM ROOT: \`make $target\` runs nothing from the repo root — remove it from the root .PHONY list, which stops it reaching backend/" >&2
    failed=1
  fi
done <<<"$advertised"

typo="no-such-target-make-target-parity"
if typo_run="$(root_make -n "$typo" 2>&1)"; then
  echo "SWALLOWED TYPO: \`make $typo\` succeeded from the repo root; an unknown name must fail" >&2
  failed=1
elif ! grep -q "No rule to make target" <<<"$typo_run"; then
  echo "SWALLOWED TYPO: \`make $typo\` failed without naming the missing target:" >&2
  printf '%s\n' "$typo_run" >&2
  failed=1
fi

if ! help_run="$(root_make help 2>&1 >/dev/null)"; then
  printf '%s\n' "$help_run" >&2
  failed=1
fi

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
echo "make target parity OK ($(grep -c . <<<"$advertised") backend targets run from the repo root, a typo fails, make help prints)"
