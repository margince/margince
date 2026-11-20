#!/usr/bin/env bash
# with-tree-digests.sh <command...> — runs the command with one TREE_DIGEST_<NAME>
# variable per top-level entry exported, the values CI takes from GITHUB_ENV.
# A test that reads outside its module reads one (gatekit.DeclareInputs) and Go's
# test cache keys on its value. Fails, without running the command, when the
# digests cannot be computed: a command that ran with none would replay stale passes.
set -euo pipefail

if [ -z "${CI:-}" ]; then
	digests="$("$(dirname "$0")/ci-stable-mtimes.sh" --export)"
	[ -n "$digests" ] || { echo "with-tree-digests: ci-stable-mtimes.sh --export printed no digests" >&2; exit 1; }
	while IFS= read -r line; do
		export "$line"
	done <<<"$digests"
fi
exec "$@"
