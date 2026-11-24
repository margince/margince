#!/usr/bin/env bash
# Refuse a `make db-up` that would recreate a RUNNING dev container.
#
# docker-compose.dev.yml pins `name: margince`, so every checkout on a machine
# shares one Postgres, Redis and MinIO. `docker compose up` recreates a running
# container whenever its service config differs from the caller's: another
# branch's compose file, or another PG_FSYNC. Every client of every other
# worktree then loses its server for a few seconds. Through the published port
# that reads as a connect that ends with no answer, never as a named error:
#
#   127.0.0.1:15432 (localhost): tls error: EOF
#   127.0.0.1:15432 (localhost): failed to receive message: unexpected EOF
#
# DB_RECREATE=1 lets the recreate happen, for the one who knows no lane or dev
# stack is using the containers. A stopped container is not checked: nothing is
# connected to it.
set -euo pipefail

[[ "${DB_RECREATE:-}" == 1 ]] && exit 0

# The compose command arrives as arguments, so the caller's shell has already
# removed its quotes: a file path with a space stays one word.
compose=("$@")
((${#compose[@]} > 0)) || compose=(docker compose -f docker-compose.dev.yml)

wanted="$("${compose[@]}" config --hash '*')"
drifted=()
while read -r service hash; do
	[[ -n "$service" ]] || continue
	container="$("${compose[@]}" ps -q --status running "$service")"
	[[ -n "$container" ]] || continue
	running="$(docker inspect -f '{{index .Config.Labels "com.docker.compose.config-hash"}}' "$container")"
	[[ "$running" == "$hash" ]] || drifted+=("$service")
done <<<"$wanted"

((${#drifted[@]} == 0)) && exit 0

echo "FAIL: db-up would recreate the running ${drifted[*]} container(s): their config differs from this checkout's docker-compose.dev.yml or environment (PG_FSYNC)." >&2
echo "  Every checkout on this machine shares them, so a recreate drops every connection of every other worktree's lane and dev stack, as 'tls error: EOF'." >&2
echo "  If nothing else is using them, run it again with DB_RECREATE=1." >&2
exit 1
