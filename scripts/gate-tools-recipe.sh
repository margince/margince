#!/usr/bin/env bash
# gate-tools-recipe.sh — print the shell recipe `make tools` runs, with its
# version pins filled in and its comment lines dropped.
#
# The gate binaries cache keys on this text, and the CI image runs it to build
# the same binaries, so a new tool or pin in backend/Makefile reaches both and
# an edit elsewhere in the Makefile reaches neither. It fails when the recipe
# shows no `go install`, because a key or an image built from an empty recipe
# would follow no pin at all.
set -euo pipefail
cd "$(dirname "$0")/../backend"

# stderr is dropped because the Makefile's parse-time `go env` calls complain
# on a machine without Go, and none of them reaches this recipe.
recipe="$(make -s -n tools 2>/dev/null | grep -v '^[[:space:]]*#')"
if ! grep -q 'go install ' <<<"$recipe"; then
	echo "gate-tools-recipe: 'make -n tools' shows no go install, so nothing can follow its pins" >&2
	exit 1
fi
printf '%s\n' "$recipe"
