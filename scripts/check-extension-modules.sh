#!/usr/bin/env bash
# check-extension-modules.sh — prove every extension module can still be tidied,
# and that tidying it changes nothing.
#
# THE FAILURE THIS REPLACES was not a broken build. A Renovate Go bump raised a
# dependency in extensions/zalo-personal/go.mod and left that module's go.sum on
# the old version, because its artifact update shells out to `go mod tidy` and
# tidy could not run there: pkg/extension has no module version, it resolves only
# through the generated build/composition/go.work, and tidy ignores workspace
# `use` directives. The committed pair was inconsistent and nothing in the tree
# noticed — the check that reported it was renovate/artifacts, which is advisory.
#
# Worse than failing, once a `replace` is absent: tidy does not stop, it goes to
# the NETWORK and pins a pseudo-version of whatever commit happened to be pushed.
# The unit then compiles against a different backend than the one it ships with,
# and the go.mod says so in a line nobody reads.
#
# So this asserts two things at once, and the second is the one that lasts: tidy
# RUNS, and it is a no-op. A module whose committed go.mod/go.sum already say what
# tidy would say is one a dependency bump can land in unattended.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
failures=0

for mod in "$root"/extensions/*/go.mod; do
    [[ -e "$mod" ]] || continue
    dir="$(dirname "$mod")"
    unit="$(basename "$dir")"

    # The local backend has to be reachable, or tidy resolves it from the network
    # and the pseudo-version it writes looks like a normal dependency.
    if ! grep -q '^replace github.com/margince/margince/backend => \.\./\.\./backend$' "$mod"; then
        printf '  FAIL %s: go.mod has no local replace for the backend — `go mod tidy` will resolve\n' "$unit" >&2
        printf '       pkg/extension from the network and pin a pseudo-version of some pushed commit.\n' >&2
        printf '       Add: replace github.com/margince/margince/backend => ../../backend\n' >&2
        failures=$((failures + 1))
        continue
    fi

    before="$(cat "$mod")"
    beforeSum=""
    [[ -f "$dir/go.sum" ]] && beforeSum="$(cat "$dir/go.sum")"

    if ! ( cd "$dir" && GOFLAGS=-mod=mod go mod tidy ) 2>/dev/null; then
        printf '  FAIL %s: `go mod tidy` does not run here, so no dependency bump can update it\n' "$unit" >&2
        failures=$((failures + 1))
        continue
    fi

    afterSum=""
    [[ -f "$dir/go.sum" ]] && afterSum="$(cat "$dir/go.sum")"
    if [[ "$before" != "$(cat "$mod")" ]] || [[ "$beforeSum" != "$afterSum" ]]; then
        printf '  FAIL %s: go.mod/go.sum are not what `go mod tidy` produces — the committed pair\n' "$unit" >&2
        printf '       disagrees with itself, which is the shape a half-applied bump leaves.\n' >&2
        printf '       Run: (cd extensions/%s && go mod tidy) and commit the result.\n' "$unit" >&2
        printf '%s' "$before" > "$mod"
        if [[ -n "$beforeSum" ]]; then printf '%s' "$beforeSum" > "$dir/go.sum"; else rm -f "$dir/go.sum"; fi
        failures=$((failures + 1))
        continue
    fi
    printf '  ok   %s: tidy runs and changes nothing\n' "$unit"
done

if [[ "$failures" -ne 0 ]]; then
    echo "FAIL: $failures extension module(s) a dependency bump cannot update unattended" >&2
    exit 1
fi
echo "OK: every extension module tidies clean"
