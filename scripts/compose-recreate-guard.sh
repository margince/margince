#!/usr/bin/env bash
# Refuse a `make db-up` that would stop or rebuild a RUNNING dev container.
#
# docker-compose.dev.yml pins `name: margince`, so every checkout on a machine
# shares one Postgres, Redis and MinIO. `docker compose up` stops or rebuilds a
# running container whenever the caller's settings differ: another branch's
# service or network settings, or another PG_FSYNC. Every client of every other
# worktree then loses its server for a few seconds. Through the published port
# that can read as a connect that ends with no answer, not a named error:
#
#   127.0.0.1:15432 (localhost): tls error: EOF
#   127.0.0.1:15432 (localhost): failed to receive message: unexpected EOF
#
# Compose's own dry run decides what would change, so a kind of drift Compose
# acts on is a kind this sees. DB_RECREATE=1 lets the change happen, for the
# one who knows no lane or dev stack is using the containers. A stopped
# container is not counted: nothing is connected to it.
set -euo pipefail

[[ "${DB_RECREATE:-}" == 1 ]] && exit 0

# The compose command arrives as arguments, so the caller's shell has already
# removed its quotes: a file path with a space stays one word.
compose=("$@")
((${#compose[@]} > 0)) || compose=(docker compose -f docker-compose.dev.yml)

if ! plan="$("${compose[@]}" up -d --dry-run 2>&1)"; then
	printf '%s\n' "$plan" >&2
	echo "FAIL: db-up could not ask compose what 'up' would change (the dry run above failed), so it will not risk the shared containers." >&2
	exit 1
fi

disrupted=()
while read -r kind name action _; do
	[[ "$kind" == Container && ("$action" == Recreate || "$action" == Stopping) ]] || continue
	[[ "$(docker inspect -f '{{.State.Running}}' "$name" 2>/dev/null)" == true ]] || continue
	[[ " ${disrupted[*]-} " == *" $name "* ]] || disrupted+=("$name")
done <<<"$plan"

((${#disrupted[@]} == 0)) && exit 0

echo "FAIL: db-up would stop or rebuild the running container(s) ${disrupted[*]}: their settings differ from this checkout's docker-compose.dev.yml or environment (PG_FSYNC)." >&2
echo "  Every checkout on this machine shares them, so this would interrupt other worktrees' lanes and dev stacks, with connection errors such as 'tls error: EOF'." >&2
echo "  If nothing else is using them, run it again with DB_RECREATE=1." >&2
exit 1
