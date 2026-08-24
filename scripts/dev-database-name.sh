#!/usr/bin/env bash
# Print the Postgres database THIS worktree's dev stack runs against.
#
# A one-line wrapper so a Makefile can ask for the name without embedding an
# absolute path in a `$(shell …)` string: make hands that string to /bin/sh, and a
# checkout path containing an apostrophe or a space breaks the quoting before the
# shell ever reaches the library. Invoked by a path relative to the Makefile, this
# has nothing to quote.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
# shellcheck source=scripts/lib-devstate.sh
. ./lib-devstate.sh
dev_database_name
