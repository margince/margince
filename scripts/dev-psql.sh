#!/usr/bin/env bash
# Run psql against the dev Postgres THIS checkout's stack talks to.
#
# WHICH container that is comes from dev-postgres-container.sh, which resolves
# it by the DSN's own port rather than by the compose project — see the reasons
# there. psql stays inside a container either way: hosts need Go and Docker
# only, not a psql client.
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: dev-psql.sh <host-port> <database> [psql args…]" >&2
  exit 2
fi
port="$1"
database="$2"
shift 2

container="$(bash "$(dirname "${BASH_SOURCE[0]}")/dev-postgres-container.sh" "$port")"
exec docker exec -i "$container" psql -U margince_owner -d "$database" "$@"
