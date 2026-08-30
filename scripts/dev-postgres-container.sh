#!/usr/bin/env bash
# The dev Postgres THIS checkout's stack talks to, by container id.
#
# The dev tooling used to address that database two ways with nothing tying
# them together:
#
#   - by COMPOSE PROJECT — `docker compose -f infra/docker-compose.dev.yml exec
#     postgres psql …`, which every ad-hoc statement went through;
#   - by PUBLISHED HOST PORT — the DSN in scripts/dev.sh, which is what the api,
#     the worker and the migrator actually connect to.
#
# infra/docker-compose.dev.yml pins `name: margince`, so every checkout of this
# repository on one machine resolves to the SAME compose project. The two paths
# therefore select the same container only while one checkout is the only one
# that has ever brought the stack up, and when they diverge nothing says so.
#
# WHAT THAT COSTS. A seed run from a second checkout landed in the first
# checkout's postgres — a container publishing no host port at all — while the
# api served the migrated database on :15432, untouched. That one failed loudly
# on its first statement because the wrong database was empty. The ordinary case
# is worse: both checkouts run the same migrations, so the wrong database is
# usually a migrated one, and the seed SUCCEEDS, writing another checkout's data.
#
# So there is one way to name it now, and it is the DSN's own port: the
# container this resolves is by construction the one the connecting binaries
# reach. When no container publishes that port, or more than one does, this
# REFUSES and says what it found, rather than picking one.
#
# psql stays inside a container either way — hosts need Go and Docker only, not
# a psql client (scripts/dev.sh's own note above psql_owner).
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: dev-postgres-container.sh <host-port>" >&2
  exit 2
fi
port="$1"

# The compose service label as well as the port: a container publishing 15432
# that is not this stack's postgres is somebody else's server, and running the
# seed inside it is the failure this exists to stop, one host over.
#
# A read loop rather than `mapfile`: this runs on whatever bash the host has,
# and macOS still ships 3.2, where mapfile does not exist. The failure was
# "command not found" mid-run, which reads like the container is missing.
found=()
while IFS= read -r line; do
  [[ -n "$line" ]] && found+=("$line")
done < <(docker ps \
  --filter "publish=${port}" \
  --filter "label=com.docker.compose.service=postgres" \
  --format '{{.ID}} {{.Names}}')

case "${#found[@]}" in
0)
  echo "FAIL: no postgres container publishes :${port}, which is the port this checkout's DSN names." >&2
  echo "  The dev stack is not up, or it is up on another port. Run 'make db-up' from this checkout." >&2
  exit 1
  ;;
1) ;;
*)
  echo "FAIL: ${#found[@]} postgres containers publish :${port}, and this cannot tell which one the DSN means:" >&2
  printf '  %s\n' "${found[@]}" >&2
  echo "  Stop the stacks you are not using ('make dev-stop' in their checkouts), then retry." >&2
  exit 1
  ;;
esac

printf '%s\n' "${found[0]%% *}"
