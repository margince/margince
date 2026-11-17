#!/usr/bin/env bash
# Core's baseline: write it, and prove it builds what the history builds.
#
#   generate  build a database from core's history through the version named in
#             backend/migrations/baseline/core/replaces.txt (or THROUGH=...),
#             dump it, and write the baseline and its manifest from the dump.
#   verify    build one database by the baseline route and one by the full
#             history, through the real migrator both times, and diff their
#             schemas, reference rows and ledgers. Empty is the only pass.
#
# Both databases come from backend/tools/migrate-route, which calls
# dbmigrate.Up: a replay in psql would run the `-- pgmigrate:no-transaction`
# files inside a transaction, which Postgres refuses, and would skip the ledger
# rows that some migrations read.
#
# pg_dump runs INSIDE the Postgres container, never on the host. A host pg_dump
# is whatever the developer's package manager installed, and a newer pg_dump
# emits SQL shaped for the newer server, so the artifact would differ per
# machine while describing one schema.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

CONTAINER="${MARGINCE_PG_CONTAINER:-margince-postgres-1}"
OWNER="${MARGINCE_PG_OWNER:-margince_owner}"
# The host's way into the same server, for the migrator.
DSN_BASE="${MARGINCE_PG_DSN_BASE:-postgres://margince_owner:dev@localhost:15432}"
BASELINE_DIR=backend/migrations/baseline/core
# Every scratch file lives under one directory, removed however the run ends.
SCRATCH="$(mktemp -d)"
trap 'rm -rf "$SCRATCH"' EXIT
TOOLS_BIN="$SCRATCH/bin"

# Tracking tables are compared on their own (dump_ledger): the baseline route
# records one row the history does not.
EXCLUDE=(--exclude-table='schema_migrations_*' --exclude-table-data='schema_migrations_*')

# NO --schema RESTRICTION. `--schema=public` drops the CREATE EXTENSION
# statements, the `ext` schema and that schema's GRANT and COMMENT, and the
# loss is invisible: a dump that omits an extension still restores fine against
# a database that already has it.

die() { echo "migration-baseline: $*" >&2; exit 1; }

require_container() {
  docker exec "$CONTAINER" true 2>/dev/null ||
    die "container '$CONTAINER' is not running — start it with 'make db-up' (override with MARGINCE_PG_CONTAINER)"
}

psql_owner() { docker exec -i "$CONTAINER" psql -U "$OWNER" -v ON_ERROR_STOP=1 -q "$@"; }

recreate_db() {
  psql_owner -d postgres -c "DROP DATABASE IF EXISTS $1" >/dev/null
  psql_owner -d postgres -c "CREATE DATABASE $1" >/dev/null
}

drop_db() { psql_owner -d postgres -c "DROP DATABASE IF EXISTS $1" >/dev/null; }

# A data-only dump warns about every table in a foreign-key cycle. Those
# warnings are expected here; anything else pg_dump says is kept.
quiet_cycle_warnings() {
  grep -vE '^pg_dump: (warning: there are circular foreign-key constraints|detail: |hint: )' >&2 || true
}

build_tools() {
  mkdir -p "$TOOLS_BIN"
  (cd backend/tools && go build -o "$TOOLS_BIN" ./migrate-route ./gen-baseline) ||
    die "building backend/tools/migrate-route and gen-baseline failed"
}

# migrate DB ARGS... — run the real migrator against DB along one route.
migrate() {
  local db="$1"; shift
  MARGINCE_OWNER_DSN="$DSN_BASE/$db" "$TOOLS_BIN/migrate-route" "$@" ||
    die "migrating $db ($*) failed"
}

# The text pg_dump writes about the dump rather than the schema: its object
# headers, the session SETs, and the \restrict guard whose token is random on
# every run.
strip_preamble() {
  sed -E '/^-- (Name|Data for Name): .*; Type: /d; /^-- (PostgreSQL database dump|Dumped (from|by) )/d; /^--$/d;
          /^SET /d; /^SELECT pg_catalog\.set_config/d; /^\\(un)?restrict /d'
}

dump_schema() {
  docker exec "$CONTAINER" pg_dump -U "$OWNER" -d "$1" --schema-only "${EXCLUDE[@]}" |
    strip_preamble | cat -s
}

# dump_data DB — the reference rows the migrations insert, from every table.
#
# TWO VALUE KINDS ARE MASKED, and only these two: a `uuidv7()` key and a
# `now()` timestamp are generated per run. Every key, label, sort order, flag
# and enum value is compared exactly. gen-baseline refuses a uuid that links
# two rows, so masking uuids hides no link.
dump_data() {
  docker exec "$CONTAINER" pg_dump -U "$OWNER" -d "$1" \
      --data-only --column-inserts "${EXCLUDE[@]}" 2> >(quiet_cycle_warnings) |
    strip_preamble |
    sed -E "s/'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}'/'<uuid>'/g;
            s/'[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?[+-][0-9]{2}'/'<timestamp>'/g" |
    LC_ALL=C sort | cat -s
}

# dump_ledger DB BASELINE_VERSION — every ledger row but the baseline's own,
# without applied_at.
dump_ledger() {
  psql_owner -d "$1" -At -c "
    SELECT 'core ' || version || ' ' || name || ' ' || coalesce(content_digest, 'NULL')
      FROM schema_migrations_core WHERE version <> '$2'
    UNION ALL
    SELECT 'custom ' || version || ' ' || name || ' ' || coalesce(content_digest, 'NULL')
      FROM schema_migrations_custom
    ORDER BY 1"
}

capture() {
  { echo "-- schema"; dump_schema "$1"; echo "-- data"; dump_data "$1"; echo "-- ledger"; dump_ledger "$1" "$2"; } > "$3"
}

manifest_field() {
  [[ -f "$BASELINE_DIR/replaces.txt" ]] || die "$BASELINE_DIR/replaces.txt is missing"
  case "$1" in
    release) awk '$1 == "release" { print $2 }' "$BASELINE_DIR/replaces.txt" ;;
    through) awk 'NF == 3 && $1 !~ /^#/ { v = $1 } END { print v }' "$BASELINE_DIR/replaces.txt" ;;
  esac
}

baseline_file() {
  local files=("$BASELINE_DIR"/*.up.sql)
  [[ ${#files[@]} -eq 1 && -f "${files[0]}" ]] || die "want exactly one .up.sql in $BASELINE_DIR"
  basename "${files[0]}" .up.sql
}

# generate — RELEASE, THROUGH, VERSION and NAME default to the baseline already
# in the tree, so a bare run rewrites it in place.
cmd_generate() {
  require_container
  local release="${RELEASE:-$(manifest_field release)}"
  local through="${THROUGH:-$(manifest_field through)}"
  local current; current="$(baseline_file)"
  local version="${VERSION:-${current%%_*}}" name="${NAME:-${current#*_}}"
  local roles=(backend/migrations/core/*_a_unit_sql_runs_as_the_ext_runtime_role.up.sql)
  [[ ${#roles[@]} -eq 1 && -f "${roles[0]}" ]] || die "want exactly one migration that makes the ext_runtime role"
  build_tools

  local db=margince_baseline_generate work="$SCRATCH/generate"
  mkdir -p "$work/out"
  echo "migration-baseline: building core's history through $through"
  recreate_db "$db"
  migrate "$db" -route history -through "$through"
  local since until
  since="$(psql_owner -d "$db" -At -c "SELECT min(applied_at) FROM schema_migrations_core")"
  until="$(psql_owner -d "$db" -At -c "SELECT max(applied_at) FROM schema_migrations_core")"
  for section in pre-data post-data; do
    docker exec "$CONTAINER" pg_dump -U "$OWNER" -d "$db" --schema-only --no-owner \
      --section="$section" "${EXCLUDE[@]}" > "$work/$section.sql"
  done
  docker exec "$CONTAINER" pg_dump -U "$OWNER" -d "$db" --data-only --column-inserts \
    --rows-per-insert=1 "${EXCLUDE[@]}" > "$work/data.sql" 2> >(quiet_cycle_warnings)
  drop_db "$db"

  # Written aside and moved in only on success, so a refusal leaves the
  # committed baseline in place.
  "$TOOLS_BIN/gen-baseline" -pre "$work/pre-data.sql" -data "$work/data.sql" -post "$work/post-data.sql" \
    -roles "${roles[0]}" -since "$since" -until "$until" -through "$through" -version "$version" \
    -name "$name" -release "$release" -out "$work/out" || die "gen-baseline refused; the baseline is unchanged"
  rm -f "$BASELINE_DIR"/*.up.sql
  mv "$work/out"/* "$BASELINE_DIR"/
  echo "migration-baseline: wrote $BASELINE_DIR/${version}_${name}.up.sql and replaces.txt; run verify next"
}

cmd_verify() {
  require_container
  build_tools
  local version; version="$(baseline_file)"; version="${version%%_*}"
  local history="$SCRATCH/history.txt" baseline="$SCRATCH/baseline.txt"

  echo "migration-baseline: building every core and custom migration by the history route"
  recreate_db margince_baseline_history
  migrate margince_baseline_history -route history
  capture margince_baseline_history "$version" "$history"

  echo "migration-baseline: building the same by the baseline route"
  recreate_db margince_baseline_route
  migrate margince_baseline_route -route baseline
  capture margince_baseline_route "$version" "$baseline"

  drop_db margince_baseline_history
  drop_db margince_baseline_route

  local diff_out="${TMPDIR:-/tmp}/margince-baseline.diff"
  if diff -u "$history" "$baseline" > "$diff_out"; then
    echo "OK: the baseline route builds the schema, reference rows and ledger the history builds"
    return 0
  fi
  echo "FAIL: the baseline route builds something the history does not" >&2
  echo "      '-' is the history, '+' is the baseline route:" >&2
  sed -n '1,200p' "$diff_out" >&2
  echo "      (full diff: $diff_out)" >&2
  return 1
}

case "${1:-}" in
  generate) cmd_generate ;;
  verify) cmd_verify ;;
  *) die "usage: migration-baseline.sh generate|verify" ;;
esac
