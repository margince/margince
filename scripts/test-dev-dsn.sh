#!/usr/bin/env bash
# test-dev-dsn.sh — prove the dev stack resolves its DSNs the way the product
# does, and that doing so did not widen what an isolated stack can reach.
#
# `make dev` passes --dsn explicitly, which OUTRANKS the environment the
# binaries would otherwise read. That is why the resolution has to happen HERE,
# and why it is worth a test: the failure it replaces was a value set in
# .env.local that looked meaningful and did nothing for the whole dev stack.
#
# What must survive honouring MARGINCE_DSN, all three from #1252:
#   1. DEV_SLUG keeps owning the database NAME. A stack that inherited the name
#      from a supplied DSN would sit on slug-derived ports in front of the BASE
#      database — two stacks that look isolated quietly sharing one.
#   2. --fresh still refuses a redirected owner DSN, compared against the
#      EFFECTIVE value rather than the literal default.
#   3. A DSN is never echoed. It carries a password.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
dev="$root/scripts/dev.sh"
failures=0

check() { # want got description
    if [[ "$1" = "$2" ]]; then
        printf '  ok   %s\n' "$3"
    else
        printf '  FAIL %s\n       want: %s\n       got:  %s\n' "$3" "$1" "$2" >&2
        failures=$((failures + 1))
    fi
}

# The helper under test, lifted from the script rather than reimplemented — a
# copy here would be a second version of production that proves nothing about it.
eval "$(sed -n '/^with_database() {/,/^}$/p' "$dev")"

echo "dev.sh: the database segment is this stack's to name"

check "postgres://u:p@h:15432/margince_dev_x" \
      "$(with_database "postgres://u:p@h:15432/margince" margince_dev_x)" \
      "a supplied DSN keeps its credentials, host and port, and takes the slug's database"

check "postgres://u:p@h:15432/margince_dev_x" \
      "$(with_database "postgres://u:p@h:15432/somebody_elses_db" margince_dev_x)" \
      "the database it named is REPLACED, never inherited"

check "postgres://u:p@h:5432/margince_dev_x?sslmode=require" \
      "$(with_database "postgres://u:p@h:5432/prod?sslmode=require" margince_dev_x)" \
      "a query string survives — MARGINCE_DSN is what a deployment fills in, and sslmode is ordinary there"

check "postgres://u:p@h:5432/margince" \
      "$(with_database "postgres://u:p@h:5432" margince)" \
      "a DSN naming no database still gets one"

check "postgresql://u:p@h:5432/margince_dev_x" \
      "$(with_database "postgresql://u:p@h:5432/prod" margince_dev_x)" \
      "postgresql:// works too — libpq accepts both spellings, so refusing one would be this script's own rule"

# Anything that cannot be redirected is refused rather than rewritten: libpq's
# key/value form has no database segment to replace, and another scheme would be
# pointed at this stack's database only to fail at the client. Both are the same
# "fails later, further from the cause" this function exists to avoid.
for bad in "host=h port=5432 dbname=prod user=u" "mysql://u:p@h:3306/prod"; do
    status=0
    out="$(with_database "$bad" margince_dev_x 2>&1)" || status=$?
    check nonzero "$([[ "$status" -ne 0 ]] && echo nonzero || echo zero)" \
          "refused rather than rewritten: ${bad%%:*}…"
    check yes "$(grep -q "must be a postgres:// or postgresql:// URL" <<<"$out" && echo yes || echo no)" \
          "and says what shape it needs: ${bad%%:*}…"
done

echo "dev.sh: resolution order, and what it must not leak"

# The order is asserted against the script's own text: running `make dev` here
# would need Docker, a database and a free :8080, and would start a real
# stack, while these checks need only the connection decisions.
check yes "$(grep -q 'OWNER_DSN="${OWNER_DSN:-${MARGINCE_OWNER_DSN:-$COMPOSE_OWNER_DSN}}"' "$dev" && echo yes || echo no)" \
      "owner: an explicit argument, else MARGINCE_OWNER_DSN, else the compose default"
check yes "$(grep -q 'APP_DSN="${APP_DSN:-${MARGINCE_DSN:-$COMPOSE_APP_DSN}}"' "$dev" && echo yes || echo no)" \
      "app: an explicit argument, else MARGINCE_DSN, else the compose default"

# The interlock refuses when the owner DSN is not the compose Postgres, because
# --fresh drops through the compose container while migrations follow the DSN.
# Comparing against a literal would let the default drift away from the value it
# is checked against, disabling the check silently.
check yes "$(grep -q '"\$OWNER_DSN" != "\$COMPOSE_OWNER_DSN"' "$dev" && echo yes || echo no)" \
      "--fresh compares the EFFECTIVE owner DSN against the compose default, not a literal"

# psql_owner resolves its container from the OWNER DSN's own port rather than
# from the compose project, because the project name is shared by every checkout
# on the machine and the port is what the api and the migrator actually connect
# to. Addressing one database two ways is how a seed ran against another
# checkout's postgres.
check yes "$(sed -n '/^psql_owner() {/,/^}$/p' "$dev" | grep -q 'dev-psql.sh "$(dsn_port "$OWNER_DSN")"' && echo yes || echo no)" \
      "psql_owner resolves its container from the owner DSN's port, not the compose project"

# That resolution can only ever reach a LOCAL container — `docker exec` has no
# other kind — so honouring MARGINCE_OWNER_DSN still cannot let a drop reach a
# database on another host. What guards the destructive path itself is the
# --fresh interlock above, which refuses outright when the owner DSN is not the
# compose one.
check yes "$(grep -q 'exec docker exec -i "$container" psql' "$root/scripts/dev-psql.sh" && echo yes || echo no)" \
      "ad-hoc SQL runs through docker exec, so a redirected DSN reaches no host but this one"

# A DSN carries a password. echo/printf must never be handed one.
leaks="$(grep -nE '^[^#]*(echo|printf)[^|]*\$(dev_owner_url|dev_app_url|OWNER_DSN|APP_DSN|MARGINCE_DSN|MARGINCE_OWNER_DSN)' "$dev" || true)"
check "" "$leaks" "no DSN is ever echoed"

# Execute the actual API launch paragraph against a process stub. This checks
# both what the API receives and what remains in its parent environment; a grep
# for the setting alone would also accept a global export to every process.
echo "dev.sh: custom fields use this stack's owner pool without changing the app role"
launch="$(awk 'BEGIN { RS="" } /\.\/bin\/api --addr/ { print; found++ }
    END { if (found != 1) exit 1 }' "$dev")"
probe="$(mktemp -d)"
trap 'rm -rf "$probe"' EXIT
mkdir -p "$probe/bin"
cat >"$probe/bin/api" <<'STUB'
#!/usr/bin/env bash
set -eu
printf '%s\n' "${MARGINCE_SCHEMA_DSN:-missing}" > schema
while [[ $# -gt 0 ]]; do
    if [[ "$1" == --dsn ]]; then printf '%s\n' "$2" > app; break; fi
    shift
done
STUB
chmod +x "$probe/bin/api"
for database in margince margince_dev_alpha; do
    for inherited in '' 'postgres://wrong:secret@other/base'; do
        (
            cd "$probe"
            if [[ -n "$inherited" ]]; then
                export MARGINCE_SCHEMA_DSN="$inherited"
            else
                unset MARGINCE_SCHEMA_DSN
            fi
            dev_owner_url="$(with_database 'postgres://owner:secret@db/base?sslmode=require' "$database")"
            dev_app_url="$(with_database 'postgres://app:secret@db/base?sslmode=require' "$database")"
            api_port=18093 MINIO_PORT=29000 blob_bucket=probe
            deploy_cfg=probe.yaml REDIS_ADDR=localhost:16379
            public_base_url_flag=(--public-base-url http://localhost:8093)
            ai_flag=() gmail_api_flags=()
            log_as() { cat > api.log; }
            eval "$launch"
            wait "$be_pid"
            printf '%s\n' "${MARGINCE_SCHEMA_DSN-}" > parent-schema
        )
        check "$(with_database 'postgres://owner:secret@db/base?sslmode=require' "$database")" \
              "$(cat "$probe/schema")" "$database: API schema pool uses the stack's owner database"
        check "$(with_database 'postgres://app:secret@db/base?sslmode=require' "$database")" \
              "$(cat "$probe/app")" "$database: ordinary API writes keep the app role"
        check "$inherited" "$(cat "$probe/parent-schema")" \
              "$database: API assignment does not export its owner credential to sibling processes"
    done
done

if [[ "$failures" -ne 0 ]]; then
    echo "FAIL: $failures dev-stack DSN expectation(s) not met" >&2
    exit 1
fi
echo "OK: the dev stack resolves DSNs through the product's own names, and still owns the database"
