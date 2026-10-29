#!/bin/sh
# Container entrypoint for cmd/worker: serve the background role as the APP role.
# Deployment-target-agnostic — every setting is read from the environment (the
# MARGINCE_* vars in docs/reference/configuration.md and programs.md); the worker resolves
# --config/--dsn/--redis/--public-base-url from their env fallbacks.
#
# The worker runs NO migrations (the api role owns that). On a cold database it
# fails its dependency probe and the orchestrator restarts it until the api has
# migrated. On a database behind this binary it starts, stays unready, and holds
# its lanes and jobs until the api has migrated.
set -eu

# Optional convenience: an env file mounted at /.env, which the worker reads
# itself through --env-file, below the environment. The DSN check below reads
# its one value from the file in a clean shell, without exporting it.
if [ -f /.env ]; then
    set -- --env-file /.env
    # shellcheck disable=SC2016
    MARGINCE_DSN="${MARGINCE_DSN:-$(env -i /bin/sh -c 'set -a; . /.env >/dev/null; printf %s "${MARGINCE_DSN:-}"')}"
fi

: "${MARGINCE_DSN:?MARGINCE_DSN is required (app role DSN, via the --dsn env fallback)}"

echo "entrypoint: starting worker…"
exec margince-worker "$@"
