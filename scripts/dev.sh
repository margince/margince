#!/usr/bin/env bash
# One-command local dev stack on the ONE shared infra: Postgres + Redis, the api
# (cmd/api), the background worker (cmd/worker — outbox relay + Surface-B runner),
# and the Vite dev server — so the SPA runs in a real browser against a live api.
# One stack per WORKTREE, so several agent sessions can run at once. The primary
# worktree serves the app on :8080 (the api behind it on :18080) against the
# shared `margince` database; every linked worktree derives its own slug from
# its directory name and claims its own database, Redis logical database, port
# pair and object bucket — no flag to remember. `DEV_SLUG=<slug>` overrides the
# derived name.
#
# MARGINCE_ENV=dev relaxes the production-only postures (an unlicensed install
# warns rather than refuses, the data reset is reachable). It does NOT switch on
# a workspace header: one installation serves one organization (ADR-0061), the
# server resolves it itself, and no request selects a tenant. localhost is a
# browser secure-context, so the Secure session cookie survives over plain
# http — no TLS front door needed.
#
# BYOK: if .env.local sets a cloud key (GEMINI_API_KEY / OPENAI_API_KEY /
# ANTHROPIC_API_KEY / OPENAI_COMPATIBLE_API_KEY), sourcing it exports the var and
# the api/worker inherit it — SelectBrain reads the key from the environment at
# boot (the routing file holds only providers), so the cold-start read-back runs
# the real model; otherwise the offline fake (--ai-fake) drives it.
#
# Credentials are NOT hardcoded: the connection URLs derive from OWNER_DSN /
# APP_DSN, which fall back to the MARGINCE_OWNER_DSN / MARGINCE_DSN the binaries
# themselves read before reaching the compose defaults — so this script carries
# no secret literal beyond the shared dev defaults, and there is one set of names
# rather than two.
#
#   scripts/dev.sh up    [slug] [--fresh]  # spin infra + db + api + FE
#   scripts/dev.sh stop  [slug] [--drop]   # stop THIS stack; --drop also drops its db
#   scripts/dev.sh sweep       [--drop]    # stop EVERY stack on the machine
set -euo pipefail
# Runtime state (logs, pids, claims) lives under dev_state_root below, one
# directory per machine rather than per worktree — keep everything this script
# writes owner-only.
umask 077

cmd="${1:-}"
slug="${2:-}"
drop=0
fresh=0
case "${3:-}" in
  --drop) drop=1 ;;
  --fresh) fresh=1 ;;
esac

cd "$(git rev-parse --show-toplevel)"

# The revision both halves of the stack are stamped with. It is the commit
# because that is what CI passes to both images, and a local stack should
# exercise the same comparison rather than a permanently-disabled one. Export it
# before `make dev` to force a mismatch on purpose — a matching pair proves only
# the quiet case, and the skew path is the one worth seeing work.
export MARGINCE_BUILD_REVISION="${MARGINCE_BUILD_REVISION:-$(git rev-parse HEAD 2>/dev/null || echo dev)}"
repo_root="$PWD"

# The compose stack's own roles, spelled once. Named because the --fresh
# interlock below compares the effective owner DSN against this exact value, and
# a default that drifted from the thing it is compared to would disable that
# check without anyone noticing it had stopped running.
COMPOSE_OWNER_DSN="postgres://margince_owner:dev@localhost:15432/margince"
COMPOSE_APP_DSN="postgres://margince_app:margince_app_dev@localhost:15432/margince"

# This stack's connection surface, resolved the way the product resolves it:
# an explicit argument, else the environment the binaries themselves read, else
# the compose default. OWNER_DSN runs migrations; APP_DSN is the non-superuser
# role the api connects as (RLS binds it).
#
# MARGINCE_OWNER_DSN / MARGINCE_DSN are consulted because they are what cmd/api,
# cmd/worker and cmd/migrate read, and this script passes --dsn explicitly, which
# OUTRANKS that environment. A value set in .env.local — which this script
# sources — was therefore inert for the entire dev stack while looking like it
# meant something. Two names for one setting, and only one of them worked.
OWNER_DSN="${OWNER_DSN:-${MARGINCE_OWNER_DSN:-$COMPOSE_OWNER_DSN}}"
APP_DSN="${APP_DSN:-${MARGINCE_DSN:-$COMPOSE_APP_DSN}}"
REDIS_PORT="${REDIS_PORT:-16379}"
# The compose MinIO backs the blobstore seam (attachments); minioadmin is the
# well-known throwaway dev credential the compose stack already ships, never a
# production secret.
MINIO_PORT="${MINIO_PORT:-29000}"

# Slug, state root and bucket come from the shared helper — three scripts need
# the same answers and dev.sh knowing them alone is how `make dev-logs` came to
# look for a file `make dev` no longer wrote.
# shellcheck source=scripts/lib-devstate.sh
. "${repo_root}/scripts/lib-devstate.sh"

# Process helpers, defined HERE rather than beside the sweep that used to be their
# only caller: the EXIT trap below names release_unconfirmed_stack, and a trap
# installed above the definition calls a function that does not exist yet. Any
# exit between the two — a DSN that will not resolve, for instance — printed
# `command not found` and left the claim behind.

kill_pids() { # pid… — TERM, then KILL whatever is still standing
  local pids=("$@") alive=()
  [[ ${#pids[@]} -gt 0 ]] || return 0
  kill "${pids[@]}" 2>/dev/null || true
  for _ in $(seq 1 10); do
    alive=()
    for p in "${pids[@]}"; do kill -0 "$p" 2>/dev/null && alive+=("$p"); done
    [[ ${#alive[@]} -eq 0 ]] && return 0
    sleep 0.5
  done
  kill -9 "${alive[@]}" 2>/dev/null || true
}
# still_ours answers whether a pid recorded in an old state file is still a
# process of ours. PIDs are recycled: a stack killed by a crash or a machine
# sleep leaves its file behind, and by the time the next `make dev` reads it
# that number can belong to anything. The pgrep paths already re-check the
# live command line before killing — a recorded pid gets the same proof.
still_ours() { # pid
  local cmd
  cmd=$(ps -o command= -p "$1" 2>/dev/null || true)
  [[ -n "$cmd" ]] || return 1
  [[ "$cmd" == *margince* || "$cmd" == *"$repo_root"* ]]
}
# is_dev_launcher reports whether a pid is a live run of THIS script. The starter
# recorded in a reservation is a shell running dev.sh, so that is what to look for
# — checking liveness alone confuses a recycled pid for a peer.
is_dev_launcher() { # pid
  local cmd
  cmd=$(ps -o command= -p "$1" 2>/dev/null || true)
  [[ -n "$cmd" ]] || return 1
  [[ "$cmd" == *dev.sh* ]]
}

# release_unconfirmed_stack — give back this run's claim, and take down whatever
# it started. Called from the EXIT trap while stack_recorded is 0, which is every
# path that does not reach a stack answering its own readiness probe.
release_unconfirmed_stack() {
  [[ "${stack_recorded:-1}" == "0" ]] || return 0
  # Never release a claim this run did not create: it belongs to a stack that was
  # already up, and taking its record away would leave it running with nothing
  # able to stop it.
  [[ "${STACK_CLAIM_PREEXISTING:-0}" == "1" ]] && return 0
  # Nor one another `make dev` is mid-way through starting. Two runs for the same
  # slug both see a pid-less reservation, and without this the one that loses the
  # port race deletes the claim the winner is still booting against.
  local owner=''
  # shellcheck disable=SC1090
  [[ -f "$(dev_state_dir "$slug")/env" ]] && owner=$(
    STARTER_PID=''
    . "$(dev_state_dir "$slug")/env"
    printf '%s' "${STARTER_PID:-}"
  )
  [[ -n "$owner" && "$owner" != "$$" ]] && return 0
  # TERM then KILL, through the helper the sweep uses: a bare TERM can leave a
  # server standing after its claim is gone, which is the orphan this is for.
  local pids=()
  local p
  for p in "${be_pid:-}" "${fe_pid:-}" "${worker_pid:-}"; do
    [[ -n "$p" ]] && pids+=("$p")
  done
  [[ ${#pids[@]} -gt 0 ]] && kill_pids "${pids[@]}"
  # The CLAIM goes; the LOG stays. Removing the directory took the log with it,
  # so a failed boot deleted the one file that said why it failed — which cost a
  # debugging cycle here before it could cost anyone else one.
  rm -f "$(dev_state_dir "$slug")/env"
}

slug="$(dev_resolve_slug "$slug")" || exit 1
if [[ -z "$slug" ]]; then
  label="dev"
  db="margince"
else
  label="dev '$slug'"
  db="margince_dev_${slug}"
fi
blob_bucket="$(dev_bucket_for_slug "$slug")"
# :8080 is THE port — the app, the thing a human opens, always and only. The api
# sits behind it at fe+10000 and the app's dev server proxies /v1 and the probes
# through, so `curl localhost:8080/v1/...` still answers and nobody has to
# remember which of two ports serves what.
#
# The primary worktree's stack is FIXED at 8080/18080 rather than claimed, and
# the claim range starts above it, so :8080 always means the same thing.
DEV_FE_PORT_MIN=8081
DEV_FE_PORT_MAX=8179
DEV_API_PORT_OFFSET=10000

# port_listeners names only the processes SERVING a port. Plain
# `lsof -ti tcp:8080` also lists everything CONNECTED to it — the
# developer's browser among them — and this sweep kills what it is given.
port_listeners() { # port
  lsof -tiTCP:"$1" -sTCP:LISTEN 2>/dev/null || true
}

# The Redis instance serves 80 logical databases in three blocks that must not
# overlap (infra/docker-compose.dev.yml says the same): 0 is the primary
# worktree's stack, 1..63 belong to the parallel integration lane one per
# package, and 64..79 are these per-worktree stacks. A stack landing in the test
# range would have its streams FLUSHDB'd mid-run by a suite that believes it
# owns the database.
DEV_REDIS_DB_MIN=64
DEV_REDIS_DB_MAX=79

# read_registry SLUG — what every OTHER recorded stack holds, and what this slug
# already holds itself. Sets TAKEN_DBS / TAKEN_PORTS (space-separated) and
# MINE_DB / MINE_PORT.
#
# Globals rather than stdout because there are four answers, and a caller that
# had to parse one string is the place the four would drift apart.
read_registry() { # slug
  local want="$1" root state_file other REDIS_DB FE_PORT
  root="$(dev_state_root)"
  TAKEN_DBS=''; TAKEN_PORTS=''; MINE_DB=''; MINE_PORT=''
  for state_file in "$root"/*/env; do
    [[ -f "$state_file" ]] || continue
    other="$(basename "$(dirname "$state_file")")"
    REDIS_DB=''; FE_PORT=''
    # shellcheck disable=SC1090
    . "$state_file"
    if [[ "$other" == "$want" ]]; then
      MINE_DB="$REDIS_DB"; MINE_PORT="$FE_PORT"
      continue
    fi
    [[ -n "$REDIS_DB" ]] && TAKEN_DBS="${TAKEN_DBS}${TAKEN_DBS:+ }${REDIS_DB}"
    [[ -n "$FE_PORT" ]] && TAKEN_PORTS="${TAKEN_PORTS}${TAKEN_PORTS:+ }${FE_PORT}"
  done
  return 0
}

# The lowest Redis database no recorded stack holds. Refuses rather than
# wrapping: a 17th concurrent stack is a situation to notice, not to paper over,
# and doubling up is the silent failure this mechanism exists to prevent.
pick_free_db() { # → a free Redis logical database, or non-zero
  local db t used
  for (( db = DEV_REDIS_DB_MIN; db <= DEV_REDIS_DB_MAX; db++ )); do
    used=0
    for t in $TAKEN_DBS; do [[ "$t" == "$db" ]] && { used=1; break; }; done
    (( used )) || { printf '%s' "$db"; return 0; }
  done
  return 1
}

# A port is free when no recorded stack claims it AND nothing is listening on
# either half of the pair. The listener check is not redundant: the registry
# knows only about OUR stacks, so a foreign process on a port would otherwise be
# handed out, bind would fail, and wait_ready would then poll that foreign
# server and report this stack ready.
pick_free_port() { # → a free frontend port, or non-zero
  local port t used
  for (( port = DEV_FE_PORT_MIN; port <= DEV_FE_PORT_MAX; port++ )); do
    used=0
    for t in $TAKEN_PORTS; do [[ "$t" == "$port" ]] && { used=1; break; }; done
    (( used )) && continue
    [[ -n "$(port_listeners "$port")" ]] && continue
    [[ -n "$(port_listeners "$(( port + DEV_API_PORT_OFFSET ))")" ]] && continue
    printf '%s' "$port"
    return 0
  done
  return 1
}

# claim_stack SLUG — reserve this slug's Redis database and port pair, and
# RECORD the reservation before anything binds.
#
# Recording inside the lock is the load-bearing part. The old code wrote its
# state file only once the servers were up, so two `make dev` runs a second
# apart both read "nothing is taken" and both picked the same database. A claim
# that is invisible until the stack is running is not a claim.
claim_stack() { # slug → "<redis_db> <fe_port>"
  local want="$1" root lock waited=0 db port
  root="$(dev_state_root)"
  lock="${root}/.claim.lock"
  mkdir -p "$root"
  # mkdir is the atomic primitive here. A stale lock from a killed run would
  # wedge every later start, so the wait is bounded and then breaks it.
  until mkdir "$lock" 2>/dev/null; do
    (( waited++ >= 50 )) && { rm -rf "$lock"; continue; }
    sleep 0.1
  done
  trap 'rm -rf "'"$lock"'"' RETURN

  read_registry "$want"
  db="$MINE_DB"; port="$MINE_PORT"
  if [[ -z "$db" ]]; then
    db="$(pick_free_db)" || {
      echo "FAIL: every Redis database ${DEV_REDIS_DB_MIN}..${DEV_REDIS_DB_MAX} is claimed by another dev stack." >&2
      echo "  Stop one (make dev-stop DEV_SLUG=<slug>), or clear them all with 'make dev-sweep'." >&2
      return 1
    }
  fi
  if [[ -z "$port" ]]; then
    port="$(pick_free_port)" || {
      echo "FAIL: no free port pair in ${DEV_FE_PORT_MIN}..${DEV_FE_PORT_MAX}." >&2
      echo "  Stop a stack (make dev-stop DEV_SLUG=<slug>), or clear them all with 'make dev-sweep'." >&2
      return 1
    }
  fi
  # Whether this slug ALREADY had a started stack recorded. `make dev` run again
  # in a worktree whose stack is up reaches here, reclaims its own numbers, and
  # then fails the port check further down — at which point the release must not
  # delete the entry, because those pids are a live stack that dev-stop still has
  # to find. Reclaiming must also not overwrite the record with a pid-less one.
  # An entry that already describes a started stack is returned as-is: rewriting
  # it would replace live pids with a pid-less reservation, and dev-stop would
  # then have nothing to kill.
  if grep -q '^BACKEND_PID=[0-9]' "${root}/${want}/env" 2>/dev/null; then
    printf '%s %s' "$db" "$port"
    return 0
  fi
  # Nor one another `make dev` is still starting. Two runs that overlap before
  # either writes its pids would otherwise both rewrite this file, and whichever
  # wrote last would own it — so the one that then failed its port check released
  # the claim the other was booting against.
  local holder=''
  if [[ -f "${root}/${want}/env" ]]; then
    holder=$(
      STARTER_PID=''
      # shellcheck disable=SC1090
      . "${root}/${want}/env"
      printf '%s' "${STARTER_PID:-}"
    )
  fi
  # A live process that is actually one of THESE. still_ours identifies a running
  # SERVER, by the margince connection string on its command line, and a starter
  # is a shell running this script — it carries neither, so still_ours rejected
  # the very launcher this guard exists to notice. A bare `kill -0` is the other
  # wrong answer: pids are recycled, and it would refuse every later `make dev`
  # for this slug once an unrelated process inherited the number.
  if [[ -n "$holder" && "$holder" != "$STACK_STARTER_PID" ]] && is_dev_launcher "$holder"; then
    echo "FAIL: another 'make dev' (pid ${holder}) is already starting the '${want}' stack. Wait for it, or stop it with 'make dev-stop'." >&2
    return 1
  fi

  mkdir -p "${root}/${want}"
  # STARTER_PID is who is booting this stack. release_unconfirmed_stack reads it
  # so a run that loses the port race cannot delete a claim another run is still
  # starting against. It is overwritten by the final state write.
  printf 'SLUG=%s\nFE_PORT=%s\nAPI_PORT=%s\nREDIS_DB=%s\nSTARTER_PID=%s\n' \
    "$want" "$port" "$(( port + DEV_API_PORT_OFFSET ))" "$db" "$STACK_STARTER_PID" \
    >"${root}/${want}/env"
  printf '%s %s' "$db" "$port"
}

if [[ -z "$slug" ]]; then
  # Fixed, not claimed: :8080 and Redis db 0 belong to the primary worktree by
  # construction, and both claim ranges start above them.
  fe_port=8080
  api_port=18080
  redis_db=0
elif [[ "$cmd" == "up" ]]; then
  # Assign first, THEN read: `read … <<<"$(claim_stack)"` reports read's status,
  # not the claim's, so a refused claim printed its FAIL and the stack carried on
  # with an empty port and an api listening on :10000.
  # Computed HERE, not inside claim_stack: that runs in a command substitution,
  # which is a subshell, so a variable it sets never reaches this shell. The first
  # version set it in there and the trap read the default — deleting the pids of a
  # stack that was still running. Same subshell trap as the swallowed claim status
  # above, one line apart.
  STACK_CLAIM_PREEXISTING=0
  recorded_be=$(
    BACKEND_PID=''
    # shellcheck disable=SC1090
    [[ -f "$(dev_state_dir "$slug")/env" ]] && . "$(dev_state_dir "$slug")/env"
    printf '%s' "${BACKEND_PID:-}"
  )
  # ALIVE, not merely recorded. A crashed stack leaves its pids in the file, and
  # treating that as a live claim would disable the release for every later failed
  # boot — the stale reservation then holds a port and a Redis database forever.
  if [[ -n "$recorded_be" ]] && still_ours "$recorded_be"; then
    STACK_CLAIM_PREEXISTING=1
  fi
  export STACK_STARTER_PID=$$
  claimed="$(claim_stack "$slug")" || exit 1
  read -r redis_db fe_port <<<"$claimed"
  api_port=$(( fe_port + DEV_API_PORT_OFFSET ))
  # A reservation that outlives a failed boot holds its port and Redis database
  # against every later stack until somebody sweeps by hand, and nothing says so
  # — the next `make dev` in a fresh worktree just gets a higher number, and the
  # block runs out sixteen stacks early. Released on any exit before the final
  # state write, which is the point the stack is actually running.
  # Released on any exit before the stack is CONFIRMED up, and the release kills
  # whatever this run started. Deleting the claim alone would leave an api or a
  # vite running that nothing records any more — an orphan holding the port the
  # claim just gave back, which is worse than the leak it was fixing.
  stack_recorded=0
  trap 'release_unconfirmed_stack' EXIT
else
  # stop and sweep READ; only `up` claims. Claiming here would mint a reservation
  # for a stack nobody asked to start, and then the caller would tear it down
  # again — a write on the path whose whole job is to clean up.
  read_registry "$slug"
  redis_db="${MINE_DB:-0}"
  fe_port="${MINE_PORT:-0}"
  api_port=$(( fe_port == 0 ? 0 : fe_port + DEV_API_PORT_OFFSET ))
fi
REDIS_ADDR="localhost:${REDIS_PORT}/${redis_db}"

# with_database DSN NAME — the same connection, pointed at a different database.
#
# The database segment is REPLACED, never inherited. DEV_SLUG owns the name
# ($db above), and a stack that took the name from a supplied DSN would sit on
# slug-derived ports in front of the BASE database — two stacks that look
# isolated quietly sharing one.
#
# A query string is carried over rather than dropped with the rest of the
# suffix. That was harmless while these were dev-only variables nobody wrote
# that way; MARGINCE_DSN is what a DEPLOYMENT fills in, where `?sslmode=require`
# is ordinary, and silently dropping it would quietly downgrade the connection.
#
# A DSN that is not a URL is refused rather than rewritten. libpq also accepts
# `host=… dbname=…`, and there is no correct way to swap a database segment that
# is not there — building something malformed from it would fail later, further
# from the cause. Nothing here echoes the DSN: it carries a password.
with_database() { # dsn name
  local dsn="$1" name="$2" query="" scheme rest
  case "$dsn" in
    *\?*) query="?${dsn#*\?}"; dsn="${dsn%%\?*}" ;;
  esac
  # Both spellings libpq itself accepts, and only those. A `mysql://` DSN would
  # otherwise be rewritten to point at this stack's database and then fail at the
  # client, which is the same "fails later, further from the cause" this function
  # refuses the key/value form to avoid.
  case "$dsn" in
    postgres://*|postgresql://*) scheme="${dsn%%://*}://"; rest="${dsn#*://}" ;;
    *)
      echo "FAIL: the DSN must be a postgres:// or postgresql:// URL so this stack can point it at ${name}; neither another scheme nor libpq's 'host=… dbname=…' form can be redirected here. Set OWNER_DSN/APP_DSN (or MARGINCE_OWNER_DSN/MARGINCE_DSN) to one." >&2
      return 1 ;;
  esac
  # Everything from the first slash on is whatever database that DSN named; the
  # authority (credentials, host, port) is the part this stack reuses.
  rest="${rest%%/*}"
  printf '%s%s/%s%s' "$scheme" "$rest" "$name" "$query"
}

dev_owner_url="$(with_database "$OWNER_DSN" "$db")"
dev_app_url="$(with_database "$APP_DSN" "$db")"

# The owner DSN reaches cmd/migrate through the environment rather than argv (it
# carries a password, and argv is world-readable), but it is assigned PER COMMAND
# below — never exported here. An export would hand the superuser credential to
# every child this script starts, and the api and worker have no use for it: the
# api connects as margince_app precisely so FORCE row-level security binds it,
# which it does not for the superuser margince_owner is in the compose stack.

# psql is NOT a host requirement (hosts need Go + Docker only): every ad-hoc
# SQL statement runs inside the compose postgres container, the same way
# `make db-init` applies scripts/db-init.sql.
#
# WHICH container is resolved from the DSN's own port, not from the compose
# project. infra/docker-compose.dev.yml pins `name: margince`, so every checkout
# on one machine resolves to the same project and `compose exec` lands in
# whichever brought the stack up first — while the api, the worker and the
# migrator connect through this DSN. Two ways to name one database, and when
# they diverged nothing said so: a seed ran against another checkout's postgres.
psql_owner() { # db [psql args…] — SQL via args or stdin
  local db="$1"; shift
  scripts/dev-psql.sh "$(dsn_port "$OWNER_DSN")" "$db" "$@"
}

# dsn_port is the host port a postgres URL names, or 5432 when it names none —
# the port postgres itself defaults to, so a DSN written without one resolves
# the container it would actually reach.
#
# A BRACKETED IPv6 authority is taken apart first. `postgres://u:p@[::1]/db`
# has colons inside the host, so the generic "text after the last colon" read
# answers `1]` — a port nothing publishes, and a resolution failure for a DSN
# that is perfectly well formed.
dsn_port() { # dsn
  local hostport="${1#*@}"
  # Everything after the authority, in the three ways a URL can start it: a
  # path, a query, or a fragment. A DSN with no database but with parameters —
  # postgres://u:p@host:5432?sslmode=disable — would otherwise read its port as
  # `5432?sslmode=disable`, which nothing publishes.
  hostport="${hostport%%/*}"
  hostport="${hostport%%\?*}"
  hostport="${hostport%%#*}"
  case "$hostport" in
  \[*\]:*) printf '%s\n' "${hostport##*:}" ;;
  \[*\]) printf '5432\n' ;;
  *:*) printf '%s\n' "${hostport##*:}" ;;
  *) printf '5432\n' ;;
  esac
}

# Under the machine-global root, not the worktree's own .tmp/ — the same reason
# the claim registry moved there. The pids land in the SAME file claim_stack
# reserved, so a sweep from any worktree can see this stack; while these were two
# different files the reservation carried ports and no pids, and the sweep read a
# directory only this worktree could see.
rundir="$(dev_state_dir "$slug")"
log="${rundir}/dev.log"
state="${rundir}/env"

# Tag every line with the process that wrote it. api, worker and Vite all append
# to one log, and once their output interleaves there is no way to recover which
# one said what — a worker sync failure reads exactly like an api request error.
#
# At debug level the tag and the severity are ALSO coloured in the file itself,
# so a plain `tail -f` on it is readable without going through `make dev-logs`.
# That is deliberately limited to debug: colour in a file is escape codes in
# `grep` output, and a normal info-level run keeps the file clean. Debug is
# already the "I am staring at this log" mode, and it is where the job queue's
# heartbeat makes an uncoloured tail hardest to read.
#
# Callers pipe through this with process substitution — `cmd > >(log_as api)`,
# never `cmd | log_as api` — because a pipeline makes $! the LAST command in it,
# and this script's $! must stay the server's own pid or dev-stop kills an awk
# instead of the api.
log_colour=0
[[ "${MARGINCE_LOG_LEVEL:-info}" == "debug" ]] && log_colour=1

# effective_flag KEY — what the api will see for a boolean key, printed as
# "true", "false", or "" when neither file names it (the compiled default).
#
# The overlay is consulted first because MARGINCE_ENV=dev makes it the winning
# layer, and a key it names decides even when it names `false`. Both files are
# grepped rather than parsed: this is a status line, and a shell YAML parser
# would be a second implementation of the loader that could disagree with it.
# seeded_ai_routing — print a config file's `seeds.ai_routing` subtree, comments
# stripped, or nothing if the file does not declare one.
#
# Scoped by INDENTATION rather than grepped whole-file: `provider:` appears under
# other sections too (a connector names one), and a flat grep would report a
# provider this stack never binds and then demand a key for it. A missing file is
# not an error — most of the callers' inputs are optional by design.
seeded_ai_routing() { # path
  [[ -f "$1" ]] || return 0
  sed 's/#.*//' "$1" | awk '
    /^[[:space:]]*ai_routing:[[:space:]]*$/ && !inblk { inblk = 1; base = match($0, /[^[:space:]]/); next }
    inblk {
      if ($0 ~ /^[[:space:]]*$/) { next }
      if (match($0, /[^[:space:]]/) <= base) { inblk = 0; next }
      print
    }
  '
}

effective_flag() { # key
  local key=$1 f value
  for f in "$dev_cfg" "$deploy_cfg"; do
    [[ -f "$f" ]] || continue
    value=$(grep -E "^[[:space:]]+${key}:[[:space:]]*(true|false)[[:space:]]*$" "$f" \
      | tail -1 | sed -E 's/.*:[[:space:]]*//; s/[[:space:]]*$//')
    if [[ -n "$value" ]]; then
      printf '%s' "$value"
      return
    fi
  done
}

log_as() { # role — tag each line of stdin and append to $log
  awk -v mode=tag -v role="$1" -v colour="$log_colour" \
    -f "${repo_root}/scripts/lib/devlog.awk" >>"$log"
}

wait_ready() { # url timeout_s — only a 2xx counts as ready (a 401/500/503 is not).
  local url="$1" timeout="$2"
  for _ in $(seq 1 "$timeout"); do
    code=$(curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || true)
    [[ "$code" =~ ^2[0-9]{2}$ ]] && return 0
    sleep 1
  done
  return 1
}

# still_running pid grace_s — the readiness check for a process that serves no
# port. The api and the FE are probed over HTTP; the worker answers nothing, so
# the only thing that can be asked about it is whether it is still there.
#
# It is asked because the worker's failure mode is exiting DURING boot — a
# config it refuses, a provider key it needs, a bus it cannot reach — and the
# grace period is what separates that from a process that is simply slow to
# settle.
#
# One sample proves only that the process existed at that instant, so the caller
# asks TWICE: once with a grace period right after launch, and once with
# grace_s 0 after the FE readiness probe has spent real time, immediately before
# the banner claims the worker is running. grace_s 0 skips the loop and takes
# the single sample below.
#
# A worker that dies after that last sample is a different problem, and not one
# a start-up script can watch for.
still_running() {
  local pid="$1" grace="$2"
  for _ in $(seq 1 "$grace"); do
    kill -0 "$pid" 2>/dev/null || return 1
    sleep 1
  done
  kill -0 "$pid" 2>/dev/null
}

# A stack that outlives the shell that started it is the worst failure mode this
# script has: an api from an earlier branch keeps answering while Vite serves the
# code you just wrote, and the app breaks in ways that look exactly like your bug.
# So a bare `make dev` does not merely check its own two ports — it claims the
# machine: every margince server process, every recorded stack, and every
# leftover per-slug database goes, and the one stack that remains is this one on
# :8080 against `margince`. `DEV_SLUG` keeps its escape hatch (it sweeps
# nothing, so an isolated env survives until the next bare `make dev`).


# Margince server processes anywhere on this machine, not just this checkout: a
# second worktree's api on :8081 owns a different database and is exactly the
# ghost this sweep exists to remove. Matched on the binary name AND a margince
# connection string, so an unrelated program called `api` is never touched.
margince_server_pids() {
  local pid cmd
  for pid in $(pgrep -f 'bin/(api|worker)|exe/(api|worker)' 2>/dev/null || true); do
    [[ "$pid" == "$$" ]] && continue
    cmd=$(ps -o command= -p "$pid" 2>/dev/null || true)
    [[ "$cmd" == *margince* ]] && echo "$pid"
  done
}

# stack_server_pids names THIS stack's api and worker wherever they came from —
# including a run whose pid the state file no longer holds.
#
# The state file records one BACKEND_PID/WORKER_PID and every `make dev`
# overwrites it, so a worker that outlived its own start is invisible to the
# only thing that would kill it. The api is caught anyway, by its port; a worker
# binds none, so nothing looked for it and they accumulated — four against one
# database, three of them stale builds. They share a River leader election, and
# the leader is what inserts the periodic jobs: an old leader renewing its lease
# schedules only the job kinds ITS binary knows, so a job kind added since is
# never enqueued at all. Every other lane keeps running, which is what makes a
# stale worker read as a broken feature rather than as a process nobody stopped.
#
# Matched on the DSN, and on the Redis address when the caller knows it. The
# database name is already one-to-one with the slug — `margince` for the primary
# worktree, `margince_dev_<slug>` for a linked one — so the DSN alone names a
# stack. The Redis address narrows it further and is passed whenever a record
# supplies it.
#
# It is OMITTED rather than guessed when there is no record. A stack whose state
# file is gone has no claim to read, and the registry's fallback for "no claim"
# is logical database 0 — the PRIMARY stack's. Passing that would ask for a
# linked worktree's database on the primary's Redis database, match nothing, and
# clean up nothing, silently: the same shape of miss this function exists to
# close.
# The DSN match ends at a WORD BOUNDARY, never mid-value. `margince` is a prefix
# of every `margince_dev_<slug>`, so a substring test run from the primary
# worktree matches every linked worktree's servers — and the primary is the one
# whose cleanup would then kill all of them at once.
stack_server_pids() { # dsn [redis_addr]
  local pid cmd
  for pid in $(pgrep -f 'bin/(api|worker)|exe/(api|worker)' 2>/dev/null || true); do
    [[ "$pid" == "$$" ]] && continue
    cmd=$(ps -o command= -p "$pid" 2>/dev/null || true)
    [[ "$cmd" == *"--dsn $1 "* || "$cmd" == *"--dsn $1" ]] || continue
    [[ -n "${2:-}" && "$cmd" != *"--redis $2 "* && "$cmd" != *"--redis $2" ]] && continue
    echo "$pid"
  done
}

# Vite resolves out of <repo>/frontend/node_modules, so its command line carries
# the worktree path — that is what distinguishes our dev server from any other
# Vite project the developer happens to be running.
vite_pids() {
  local pid cmd root roots=()
  # EVERY worktree of this repository, enumerated. Each resolves vite out of its
  # own node_modules, so its command line carries its own path — matching on this
  # checkout's root found only our own, and `make dev-sweep` promises to clear the
  # machine. Deriving a parent directory from the git common dir was no better: a
  # linked worktree can live anywhere, not only beside its siblings.
  while IFS= read -r root; do
    [[ -n "$root" ]] && roots+=("$root")
  done < <(git worktree list --porcelain | awk '/^worktree /{print substr($0, 10)}')
  for pid in $(pgrep -f 'vite' 2>/dev/null || true); do
    [[ "$pid" == "$$" ]] && continue
    cmd=$(ps -o command= -p "$pid" 2>/dev/null || true)
    for root in "${roots[@]}"; do
      # "$root/" — with the separator. A bare substring match sweeps an unrelated
      # project living at <root>-scratch, whose path merely starts with ours.
      if [[ "$cmd" == *"$root/"* ]]; then
        echo "$pid"
        break
      fi
    done
  done
}



sweep_stacks() { # kill every margince dev stack: recorded, orphaned, or foreign
  local victims=() pids p port state_file
  local BACKEND_PID FE_PID WORKER_PID API_PORT FE_PORT
  # 1. Every stack this script ever recorded — its own pids and its own ports.
  #    The locals above shadow what the state file sets, so sourcing one cannot
  #    leak a stale pid into the rest of the run.
  for state_file in "$(dev_state_root)"/*/env; do
    [[ -f "$state_file" ]] || continue
    BACKEND_PID=''; FE_PID=''; WORKER_PID=''; API_PORT=''; FE_PORT=''
    # shellcheck disable=SC1090
    . "$state_file"
    for p in "$BACKEND_PID" "$FE_PID" "$WORKER_PID"; do
      [[ -n "$p" ]] && still_ours "$p" && victims+=("$p")
    done
    for port in "$API_PORT" "$FE_PORT"; do
      [[ -n "$port" ]] || continue
      for p in $(port_listeners "$port"); do victims+=("$p"); done
    done
  done
  # 2. Orphans whose state file is gone, and stacks from other checkouts.
  for p in $(margince_server_pids) $(vite_pids); do victims+=("$p"); done
  # 3. Anything at all holding the ports this stack is about to bind — a foreign
  #    process on :8080 loses the port rather than silently shadowing the api.
  for port in "$api_port" "$fe_port"; do
    for p in $(port_listeners "$port"); do victims+=("$p"); done
  done

  # Deduplicate: the same pid legitimately arrives from several sources.
  pids=$(printf '%s\n' "${victims[@]+"${victims[@]}"}" | grep -E '^[0-9]+$' | sort -u || true)
  if [[ -n "$pids" ]]; then
    # shellcheck disable=SC2086
    kill_pids $pids
    echo "dev: swept $(printf '%s\n' $pids | wc -l | tr -d ' ') stray process(es) from earlier stacks"
  fi
  rm -rf "$(dev_state_root)"/*
}

drop_stray_dev_dbs() { # every margince_dev_<slug> database an isolated env left behind
  local strays
  strays=$(psql_owner postgres -tAc \
    "SELECT datname FROM pg_database WHERE datname LIKE 'margince\\_dev\\_%'" 2>/dev/null | tr -d '\r' || true)
  [[ -n "$strays" ]] || return 0
  while read -r stray; do
    [[ -n "$stray" ]] || continue
    # WITH (FORCE) terminates a connection the just-killed process has not
    # finished closing; the shared `margince` and the test lane's
    # margince_test* / margince_it_* namespaces never match this pattern.
    # </dev/null is load-bearing: psql_owner runs `docker compose exec -T`,
    # which would otherwise swallow the rest of this loop's input and drop
    # exactly one database however many are stray.
    psql_owner postgres -c "DROP DATABASE IF EXISTS \"${stray}\" WITH (FORCE)" >/dev/null 2>&1 </dev/null || true
    echo "dev: dropped stray database ${stray}"
  done <<<"$strays"
}

case "$cmd" in
up)
  # `make dev` starts THIS worktree's stack and touches nobody else's. It used
  # to sweep the machine first — every margince process, every recorded stack,
  # every per-slug database — which made one engineer's routine command destroy
  # every parallel session's stack, including the one another agent was
  # mid-test against. The sweep is now `dev-sweep`, asked for by name.
  #
  # A bound port must still stop the boot: binding would fail silently and
  # wait_ready would then read "ready" off the OLD server. (Vite without
  # --strictPort would not even fail — it would walk to a port we never poll.)
  for _p in "$api_port" "$fe_port"; do
    if [[ -n "$(port_listeners "$_p")" ]]; then
      echo "FAIL: port :${_p} already in use — is $label already running?" >&2
      echo "  Stop it:                      make dev-stop${slug:+ DEV_SLUG=$slug}" >&2
      echo "  Or clear every stack here:    make dev-sweep" >&2
      exit 1
    fi
  done
  # The FE runs via `pnpm exec vite`, which needs node_modules.
  if [[ ! -d frontend/node_modules ]]; then
    echo "FAIL: frontend/node_modules missing — run 'make install' (or 'cd frontend && pnpm install') before 'make dev'." >&2
    exit 1
  fi
  mkdir -p "$rundir"
  : > "$log"
  echo "$label → db=$db api=:$api_port fe=:$fe_port (logs: $log)"
  {
    echo "=== infra + db ==="
    make db-up
    # --fresh means "the install a first customer gets": drop the database
    # and let the migrations and the bootstrap rebuild it. Deliberately not
    # the default — a restart to pick up a backend change must not cost the
    # records you were half-way through creating.
    if [[ "$fresh" == "1" ]]; then
      # psql_owner always talks to the COMPOSE Postgres, while the migration
      # below connects through OWNER_DSN. Point that elsewhere and --fresh
      # would erase one database and migrate another; refuse rather than
      # rebuild something the caller never named.
      if [[ "$OWNER_DSN" != "$COMPOSE_OWNER_DSN" ]]; then
        # The DSN itself is never echoed: it carries a password, and this
        # branch exists precisely because the caller supplied a real one.
        echo "FAIL: --fresh rebuilds the compose Postgres, but OWNER_DSN points somewhere else — drop that database yourself, then run make dev" >&2
        exit 1
      fi
      psql_owner postgres -c "DROP DATABASE IF EXISTS \"${db}\" WITH (FORCE)" </dev/null
      psql_owner postgres -c "CREATE DATABASE \"${db}\"" </dev/null
      psql_owner "$db" -v ON_ERROR_STOP=1 <scripts/db-init.sql
    fi
    # The base `margince` db already exists (db-up + db-init); only a slugged
    # env needs its own database created.
    [[ -n "$slug" ]] && psql_owner postgres -c "CREATE DATABASE \"${db}\"" 2>&1 || true
    # The composed workspace (ADR-0069): materialize build/composition/
    # and build the role binaries against it, so an enabled extension set
    # under extensions/ reaches the dev stack; vanilla composes empty.
    #
    # gen-composition runs BEFORE the migration, not after it as it used to.
    # cmd/migrate applies each enabled unit's migrations from the composed
    # set, so it needs the same workspace cmd/api is built against — and it
    # needs it to be current. Migrating first and composing after would
    # migrate from a stale composition, or from none at all on a fresh
    # checkout; migrating under the ROOT workspace (which is what a bare
    # `go run` does) resolves the vanilla stub and applies zero extension
    # migrations, leaving a composed api booting over a database with none
    # of its extensions' tables.
    ( cd backend && GOWORK="$PWD/../go.work" go run ./tools/gen-composition )
    ( cd backend && MARGINCE_OWNER_DSN="$dev_owner_url" \
      GOWORK="$PWD/../build/composition/go.work" go run ./cmd/migrate up )
    echo "=== build api (once, before the readiness poll) ==="
    # ONE revision for both halves of the stack, so the api and the documents it
    # fetches can be compared exactly as they are in a deployed installation.
    # Overridable: setting MARGINCE_BUILD_REVISION before `make dev` is how the
    # skew path is exercised deliberately, since a matching pair proves only the
    # quiet case.
    ( cd backend && GOWORK="$PWD/../build/composition/go.work" \
        go build -ldflags "-X github.com/margince/margince/backend/internal/shared/buildinfo.Revision=${MARGINCE_BUILD_REVISION}" \
        -o ../bin/api ./cmd/api )
    echo "=== servers ==="
  } > >(log_as boot) 2>&1

  # The model binding is a stored setting, planted at bootstrap from
  # `seeds.ai_routing`; there is no routing file to seed any more. Which file
  # declares it decides which providers this stack binds, and the layering picks
  # the winner: the posture overlay is applied AFTER the base, so if
  # config/margince.dev.yaml declares the key it wins over the engineer's own
  # config/margince.yaml.
  #
  # Scanned only to choose between a real model and the offline fake below. The
  # binding itself is never read from a file by the api or the worker.
  routing_src="config/margince.yaml"
  if seeded_ai_routing config/margince.dev.yaml | grep -q .; then
    routing_src="config/margince.dev.yaml"
  fi

  # BYOK: the real model powers the /coldstart read-back when a cloud key is in
  # the environment, the offline fake otherwise. Secrets ride the ENVIRONMENT —
  # the api resolves each provider's key from its conventional env var
  # (GEMINI_API_KEY / OPENAI_API_KEY / ANTHROPIC_API_KEY / OPENAI_COMPATIBLE_API_KEY)
  # at boot; the routing file names only providers, never a key. Sourcing
  # .env.local exports those vars, and the api/worker started below inherit them —
  # no key ever lands in a config file. Seed .env.local from the tracked template
  # on first run so a fresh clone has a documented place for these keys.
  if [[ ! -f .env.local && -f .env.example ]]; then
    cp .env.example .env.local
    echo "dev: seeded .env.local from .env.example — edit it to set keys (GEMINI_API_KEY, MARGINCE_GMAIL_*, …)"
  fi
  ai_flag=(--ai-fake)
  if [[ -f .env.local ]]; then
    set -a; . ./.env.local; set +a
  fi
  # Real routing needs the key for EVERY cloud provider the routing file
  # actually binds — SelectBrain fails closed at boot on the first bound
  # provider whose env key is missing, so "any key present" is not enough
  # (e.g. an anthropic-only .env.local against the gemini-bound template
  # would refuse to start). Comments are stripped before scanning so the
  # template's commented alternatives don't count as bindings.
  # Quotes stripped before matching: `provider: "gemini"` is valid YAML, and a
  # pattern anchored on a bare word skipped it — which left bound_providers
  # empty, missing_keys empty, and real mode selected with no key at all.
  bound_providers=$(seeded_ai_routing "$routing_src" \
    | tr -d "\"'" \
    | grep -Eo 'provider:[[:space:]]*[a-z_]+' \
    | awk -F': *' '{print $2}' | sort -u || true)
  missing_keys=""
  for _p in $bound_providers; do
    _env=""
    case "$_p" in
      anthropic)         _env="ANTHROPIC_API_KEY" ;;
      openai)            _env="OPENAI_API_KEY" ;;
      gemini)            _env="GEMINI_API_KEY" ;;
      openai_compatible) _env="OPENAI_COMPATIBLE_API_KEY" ;;
    esac
    if [[ -n "$_env" && -z "${!_env:-}" ]]; then
      missing_keys="$missing_keys $_env"
    fi
  done
  # THREE outcomes, and collapsing the first two is how this lied: "no binding
  # declared" is not "every bound provider is satisfied", but both leave
  # missing_keys empty.
  #
  # --ai-fake is kept in the first two cases rather than dropped. It is inert
  # against a servable stored binding — the api prefers what the database holds
  # — so passing it costs a bound installation nothing, and it is what makes the
  # AI surfaces answer at all when the binding is absent or unbuildable.
  if [[ -z "$bound_providers" ]]; then
    echo "dev: $routing_src declares no model binding, so the AI lanes answer from the offline fake. Declare one under seeds.ai_routing and \`make dev-fresh\`, or bind models under Settings -> AI on the running stack"
  elif [[ -n "$missing_keys" ]]; then
    echo "dev: $routing_src binds provider(s) whose key is not set (${missing_keys# }) — the AI lanes answer from the offline fake; set the key(s) in .env.local or rebind the provider in $routing_src"
  else
    # Every bound provider is satisfied, so the stored binding serves and the
    # flag above never comes into play. It was planted from $routing_src at
    # BOOTSTRAP, so a database created before that file declared a binding still
    # has none — `make dev-fresh` re-runs the bootstrap, and Settings -> AI binds
    # a running stack without one.
    ai_flag=()
    echo "dev: the stored model binding serves the cold-start read-back (providers bound by $routing_src: $(echo $bound_providers | tr '\n' ' '))"
  fi

  # The dev keyvault seals connector credentials (IMAP app passwords, OAuth
  # refresh tokens). Export it unconditionally so `make dev` can connect an IMAP
  # mailbox with no Google OAuth app configured — the gmail branch below adds
  # only the OAuth-specific state key and base URLs.
  export MARGINCE_KEYVAULT_ROOT_KEY="${MARGINCE_KEYVAULT_ROOT_KEY:-bWFyZ2luY2UtZGV2LW9ubHkta2V5dmF1bHQtcm9vdGs=}"

  # The SPA's own origin (:fe_port) is where a browser lands — the same origin
  # Gmail consent redirects to and, per DESIGN §5.1, the same origin an MCP
  # client is told to use (http://localhost:${fe_port}/mcp). Passed
  # unconditionally, not just when Gmail is configured: cmd/api refuses to
  # boot with mcp.connector_enabled=true and no --public-base-url (the OAuth
  # audience and the advertised MCP resource must never come from the Host
  # header), so an engineer who flips that gate needs this flag present with
  # no Gmail env vars in sight.
  # A tunnelled host (cloudflared quick tunnel) needs this to be the public
  # URL, not localhost, or the advertised MCP resource mismatches and OAuth
  # token exchange fails. Overridable from .env.local (sourced above) so every
  # `make dev` boots tunnel-correct; unset, dev is unchanged.
  public_base_url_flag=(--public-base-url "${MARGINCE_PUBLIC_BASE_URL:-http://localhost:${fe_port}}")

  # Gmail capture connector: when .env.local supplies a Google OAuth app, pass
  # its flags to the api and run the sync worker. Absent it, `make dev` is
  # unchanged and the /connectors/gmail/* surface stays its declared 501.
  gmail_api_flags=()
  gmail_enabled=0
  if [[ -n "${MARGINCE_GMAIL_CLIENT_ID:-}" && -n "${MARGINCE_GMAIL_CLIENT_SECRET:-}" ]]; then
    gmail_enabled=1
    # Secrets travel via the environment, NEVER CLI flags (argv is visible in
    # the process table). The client id/secret are already exported from
    # .env.local; export the OAuth state key too. The api/worker flags
    # default to these env vars, so we pass only the non-secret, dev-computed
    # URL on the command line — api base = the api (:api_port), where the
    # callback redirect_uri resolves. public-base-url is Gmail's consent
    # landing origin too, but it rides on $public_base_url_flag above so it
    # is passed exactly once.
    export MARGINCE_CONNECTOR_STATE_KEY="${MARGINCE_CONNECTOR_STATE_KEY:-margince-dev-connector-state-key-0001}"
    gmail_api_flags=(
      --api-base-url "http://localhost:${api_port}"
    )
    echo "dev: gmail capture connector enabled (callback http://localhost:${api_port}/v1/connectors/gmail/callback)"
  fi

  # The deployment configuration (A107/ADR-0061): the api bootstraps the demo
  # organization itself at boot — no public provisioning endpoint exists. Seeded
  # ONCE into a gitignored config/margince.yaml from config/margince.example.yaml
  # and then LEFT ALONE (create-if-missing / leave-if-exists) — so an engineer
  # can edit org details or runtime
  # posture (e.g. ai.capture_payloads for Layer-3 capture) and it persists across
  # restarts (it lives in config/, not the scratch rundir dev-stop clears).
  deploy_cfg="config/margince.yaml"
  dev_cfg="config/margince.dev.yaml"
  admin_pw_file="config/margince-admin-password"
  # The OPERATOR's password, which is deliberately not the one anybody signs in
  # with. A configured bootstrap requires the admin to replace it before the
  # account works at all, so this value's whole life is the first login;
  # `make seed-dev` performs that replacement and lands on the documented
  # demo-password-123. Making them the same string would hide the step that
  # every real configured installation has to take.
  if [[ ! -f "$admin_pw_file" ]]; then
    printf '%s' "${BOOTSTRAP_PASSWORD:-operator-supplied-first-password}" >"$admin_pw_file"
    chmod 600 "$admin_pw_file"
  fi
  if [[ ! -f "$deploy_cfg" ]]; then
    cp config/margince.example.yaml "$deploy_cfg"
    echo "dev: seeded $deploy_cfg from config/margince.example.yaml — edit it to change org/admin or AI posture (e.g. ai.capture_payloads)"
  fi
  # The dev posture's own differences — the Reset data button among them — live
  # in the TRACKED config/margince.dev.yaml, which MARGINCE_ENV=dev selects on
  # top of the file above. They used to be appended here, which meant the most
  # destructive capability a dev stack has was armed by a heredoc no review ever
  # saw as a diff, in a file git does not track.
  # Report which deployment config the api + worker are using, and its AI
  # posture — mirrors the ai-routing line above so both configs are visible.
  #
  # Both keys are read the way the api reads them: the dev overlay first,
  # because it wins, and the base only for a key the overlay is silent about. A
  # status line that reported the base alone would contradict the running
  # process the moment anyone used the overlay for what it is for.
  if [[ "$(effective_flag capture_payloads)" == "true" ]]; then
    capture_note="ai.capture_payloads ON"
  else
    capture_note="ai.capture_payloads off"
  fi
  echo "dev: using $deploy_cfg (+ $dev_cfg) for the deployment config ($capture_note)"
  # An operator flipping mcp.connector_enabled needs to see that it took
  # effect — the gate is otherwise silent until a client actually tries it.
  if [[ "$(effective_flag connector_enabled)" == "true" ]]; then
    echo "dev: mcp.connector_enabled ON — connect a client at http://localhost:${fe_port}/mcp"
  else
    echo "dev: mcp.connector_enabled off"
  fi

  # THE FE STARTS FIRST, and the order is load-bearing rather than incidental.
  # The api reads its MCP App view documents from this origin at boot; started
  # after the api, it could never answer that read, and the advertised set is
  # frozen once the api gives up — so both views were permanently unadvertised
  # in every dev stack. Measured on a live run, not predicted.
  #
  # Nothing about vite needs the api first: the /v1 proxy resolves per request
  # (BACKEND_PORT is configuration, not a live dependency), so a request that
  # arrives before the api is up simply fails and is retried by the browser.
  #
  # `pnpm --dir frontend` keeps the cwd at the repo root, so $! is vite itself
  # (a `(cd … & )` subshell would capture the subshell, not the server).
  #
  # MARGINCE_COMPOSITION_FRONTEND is the runtime half of the composition alias,
  # and `make dev` must set it for the same reason it builds the api under the
  # composed GOWORK above: this stack IS the composed installation. Without it
  # the api served an enabled unit's routes while the SPA resolved the
  # empty-tree registry, so #/ext/<unit> answered "no extension named …" on the
  # one command the docs tell a developer to run — the whole frontend surface of
  # the tier was unreachable locally, and only the web image build set the variable.
  # Found by Task 14's UAT (F3).
  #
  # The directory is always present here: gen-composition runs in the boot
  # block above, and vanilla composes an empty registry rather than no file.
  BACKEND_PORT="${api_port}" MARGINCE_BUILD_REVISION="${MARGINCE_BUILD_REVISION}" \
  MARGINCE_COMPOSITION_FRONTEND="$PWD/build/composition/frontend" \
    pnpm --dir frontend exec vite --port "${fe_port}" --strictPort > >(log_as fe) 2>&1 &
  fe_pid=$!

  # Run the compiled binary directly (not `go run`): it starts in <1s so the
  # poll window is real, and $be_pid is the actual server process for a clean
  # kill. Redis is the ONE shared instance. The api keeps its default inline
  # relay: it coexists with the worker's standalone relay (started below) —
  # outbox rows are claimed FOR UPDATE SKIP LOCKED, so two relays never
  # double-ship.
  MARGINCE_ENV=dev \
    MARGINCE_BLOBSTORE_ENDPOINT="localhost:${MINIO_PORT}" \
    MARGINCE_BLOBSTORE_ACCESS_KEY=minioadmin \
    MARGINCE_BLOBSTORE_SECRET_KEY=minioadmin \
    MARGINCE_BLOBSTORE_BUCKET="$blob_bucket" \
    MARGINCE_BLOBSTORE_REGION=us-east-1 \
    ./bin/api --addr ":${api_port}" --dsn "$dev_app_url" --config "$deploy_cfg" \
    --redis "${REDIS_ADDR}" \
    "${public_base_url_flag[@]}" \
    "${ai_flag[@]+"${ai_flag[@]}"}" "${gmail_api_flags[@]+"${gmail_api_flags[@]}"}" > >(log_as api) 2>&1 &
  be_pid=$!

  if ! wait_ready "http://localhost:${api_port}/readyz" 90; then
    echo "FAIL: $label api did not become ready — see ${log}" >&2
    # The FE too: it is started BEFORE the api now (the api reads its view
    # documents from it), so bailing out here would leave vite holding the port
    # and the next `make dev` would report it as already in use.
    kill "$be_pid" "$fe_pid" 2>/dev/null || true
    exit 1
  fi
  # No demo records: `make dev` brings up a COLD START — the installation the
  # api bootstrapped from the deployment config (one organization, one admin
  # seat) and nothing else, so onboarding, empty states, and first-run flows are
  # what a developer sees by default. Demo data is an explicit opt-in step:
  # `make seed-dev` (API records + the FX/RBAC fixture) jumps over the cold
  # start on a stack that is already up.

  # The background process role (cmd/worker) always runs alongside the api in
  # dev: the standalone outbox relay (coexists with the api's inline relay —
  # rows are claimed FOR UPDATE SKIP LOCKED, so two relays never double-ship),
  # the Surface-B runner scheduler, and the retention / close-date / reconcile
  # sweeps AND the automation time-scan (the ONLY cg:workflows consumer + the
  # clock-trigger scheduler — without the worker, `make dev` fires no
  # automations at all, event- or clock-triggered). It gets the SAME config
  # surface as the api — the same $ai_flag (real cloud model when
  # .env.local set a BYOK key, else the offline fake, so its runner
  # matches the api), the same blobstore endpoint, and the .env.local keys
  # already exported into this shell (vault + Gmail secrets travel via the
  # environment, never CLI flags). Gmail adds a short sync poll only when the
  # connector is configured.
  #
  # --retention-interval 720h: the worker runs the nightly GDPR
  # retention/erasure pass unconditionally — it is the River schedule of the
  # privacy_retention dispatcher, which fans out one job per workspace.
  # RunOnStart still fires one fan-out immediately at boot (inherent, not gated
  # by this flag) — but it only ERASES data past its jurisdiction floor, so on
  # a fresh dev database it is a no-op. The long interval just stops it
  # recurring during a dev session.
  ( cd backend && GOWORK="$PWD/../build/composition/go.work" go build -o ../bin/worker ./cmd/worker ) > >(log_as boot) 2>&1
  worker_gmail_flags=()
  if [[ "$gmail_enabled" == "1" ]]; then
    # A short poll makes the demo mailbox responsive; the default is 2m.
    worker_gmail_flags=(--gmail-sync-interval 30s)
  fi
  MARGINCE_ENV=dev \
    MARGINCE_BLOBSTORE_ENDPOINT="localhost:${MINIO_PORT}" \
    MARGINCE_BLOBSTORE_ACCESS_KEY=minioadmin \
    MARGINCE_BLOBSTORE_SECRET_KEY=minioadmin \
    MARGINCE_BLOBSTORE_BUCKET="$blob_bucket" \
    MARGINCE_BLOBSTORE_REGION=us-east-1 \
    ./bin/worker --dsn "$dev_app_url" --redis "${REDIS_ADDR}" \
    --config "$deploy_cfg" \
    --retention-interval 720h \
    "${ai_flag[@]+"${ai_flag[@]}"}" "${worker_gmail_flags[@]+"${worker_gmail_flags[@]}"}" > >(log_as worker) 2>&1 &
  worker_pid=$!
  # A dead worker is indistinguishable from a broken feature, which is what
  # makes it expensive: every queue-backed lane still ACCEPTS work, durably and
  # with no error, and simply never runs it. A record arrives, the row says
  # waiting, the timeline stays empty, and the obvious readings — a bad
  # signature, a broken drain, a bug in the pipeline — are all wrong and each
  # costs an afternoon to rule out. The stack has to say so itself, because the
  # only visible evidence is a process that is not in the list.
  if ! still_running "$worker_pid" 3; then
    echo "FAIL: $label worker exited during boot — no queued job will ever run" >&2
    echo "  Its own reason is in the log, under the 'worker' role:" >&2
    echo "    make dev-logs${slug:+ DEV_SLUG=$slug} ROLE=worker" >&2
    echo "    ${log}" >&2
    kill "$be_pid" "$fe_pid" 2>/dev/null || true
    exit 1
  fi
  if [[ "$gmail_enabled" == "1" ]]; then
    echo "  worker   background relay + Surface-B runner + time-scan + Gmail sync (poll every 30s)"
  else
    echo "  worker   background relay + Surface-B runner + automation time-scan running"
  fi

  # REDIS_DB is recorded because claim_redis_db reads it back: it is how this
  # slug reclaims its own index on a restart, and how the next slug knows the
  # index is spoken for.
  printf 'SLUG=%s\nAPI_PORT=%s\nFE_PORT=%s\nDB=%s\nREDIS_DB=%s\nBACKEND_PID=%s\nFE_PID=%s\nWORKER_PID=%s\nLOG=%s\n' \
    "$slug" "$api_port" "$fe_port" "$db" "$redis_db" "$be_pid" "$fe_pid" "$worker_pid" "$log" >"$state"
  if wait_ready "http://localhost:${fe_port}/" 90; then
    # Asked AGAIN, because the first check only proved the worker was alive
    # three seconds in. The FE probe above has since spent real time, and a
    # worker whose boot fails later — a slow connection, a migration it waits
    # on — would otherwise be announced as running. This is the last moment
    # before the banner claims it, so it is the last chance to be honest about
    # it. A worker that dies after this point is a different problem and not
    # one a start-up script can watch for.
    if ! still_running "$worker_pid" 0; then
      echo "FAIL: $label worker exited during boot — no queued job will ever run" >&2
      echo "  Its own reason is in the log, under the 'worker' role:" >&2
      echo "    make dev-logs${slug:+ DEV_SLUG=$slug} ROLE=worker" >&2
      echo "    ${log}" >&2
      kill "$be_pid" "$fe_pid" 2>/dev/null || true
      exit 1
    fi
    # CONFIRMED up, and only here. This used to be set at the state write above,
    # which is before this probe — so an FE that never came ready left a claim
    # behind holding a port and a Redis database, while the script killed the
    # processes and exited 1.
    stack_recorded=1
    echo "$label ready"
    echo ""
    echo "  OPEN     http://localhost:${fe_port}"
    echo ""
    echo "  api      http://localhost:${api_port}  (also proxied at :${fe_port}/v1)"
    # Printed for a slugged stack only, and printed at all because a shared bus
    # is invisible until something goes missing: a reader debugging a consumer
    # needs to know which index to point redis-cli at.
    if [[ -n "$slug" ]]; then
      echo "  bus      redis db ${redis_db} on :${REDIS_PORT}  (this slug's own — events are not shared with other stacks)"
    fi
    # The only seat on a cold start is the bootstrap admin, and the deployment
    # config is where it is defined — read the address back from that file
    # rather than restating it, so an edited config prints the truth.
    admin_email="$(sed -n 's/^[[:space:]]*email:[[:space:]]*\(.*\)$/\1/p' "$deploy_cfg" | head -1)"
    echo "  login    ${admin_email} / $(cat "$admin_pw_file")  (bootstrap admin — cold start, no other data)"
    echo "  demo     make seed-dev  — adds the demo records + rep seats on top"
    # Name the reading tool, not just the file: the raw log interleaves three
    # processes, and `make dev-logs` is what makes that legible.
    echo "  logs     make dev-logs${slug:+ DEV_SLUG=$slug}  — coloured per process (ROLE=/LEVEL=/ALL=1 to filter)"
    echo "           ${log}"
    echo "  stop     make dev-stop${slug:+ DEV_SLUG=$slug}"
  else
    echo "FAIL: $label FE did not become ready in time — see ${log}" >&2
    # Don't leave the api (and vite) orphaned when the FE readiness poll fails.
    kill "$be_pid" "$fe_pid" ${worker_pid:+"$worker_pid"} 2>/dev/null || true
    exit 1
  fi
  ;;

stop)
  # This worktree's stack, and only it. `dev-sweep` is what clears the machine.
  if [[ -f "$state" ]]; then
    # shellcheck disable=SC1090
    . "$state"
    victims=()
    for p in "${BACKEND_PID:-}" "${FE_PID:-}" "${WORKER_PID:-}"; do
      [[ -n "$p" ]] && victims+=("$p")
    done
    # Backstop: free the recorded ports by listener (reaps vite, pnpm's child).
    for p in "${API_PORT:-}" "${FE_PORT:-}"; do
      [[ -n "$p" ]] || continue
      for q in $(port_listeners "$p"); do victims+=("$q"); done
    done
    # And this stack's servers whatever their pid — the state file holds ONE
    # worker and every `make dev` overwrites it, so an earlier run's worker is
    # reachable by nothing else. It binds no port, so the port backstop above
    # misses it too; stack_server_pids says what that cost.
    for q in $(stack_server_pids "$(with_database "$APP_DSN" "${DB:-$db}")" \
                                 "localhost:${REDIS_PORT}/${REDIS_DB:-0}"); do
      victims+=("$q")
    done
    # kill_pids, not a bare `kill`: TERM alone leaves a server that ignores it
    # standing after its record is gone, which is the orphan this is for.
    stopped=$(printf '%s\n' "${victims[@]+"${victims[@]}"}" | grep -E '^[0-9]+$' | sort -u || true)
    # shellcheck disable=SC2086
    [[ -n "$stopped" ]] && kill_pids $stopped
    rm -rf "$rundir"
    echo "stopped $label (freed :${API_PORT:-?} :${FE_PORT:-?})"
  else
    # No claim, which does not mean no processes: the record is what a crash or
    # an interrupted boot loses first, and its servers keep running against this
    # database with nothing left pointing at them. Ports are CLAIMED now rather
    # than derived from the slug, so there is nothing to guess at there — but the
    # DATABASE is the slug's, so the servers can still be named.
    # No Redis address: see stack_server_pids. Without a claim, REDIS_ADDR here
    # names logical database 0 whatever the slug, which is the primary stack's.
    orphans=$(stack_server_pids "$(with_database "$APP_DSN" "$db")" | sort -u || true)
    if [[ -n "$orphans" ]]; then
      # shellcheck disable=SC2086
      kill_pids $orphans
      echo "stopped $label ($(printf '%s\n' $orphans | wc -l | tr -d ' ') process(es) whose claim was gone)"
    else
      echo "no recorded stack for $label — nothing to stop"
    fi
  fi
  if [[ "$drop" == "1" ]]; then
    if [[ -z "$slug" ]]; then
      echo "refusing to drop the shared 'margince' database — pass DEV_SLUG=<slug> to drop an isolated env" >&2
    else
      # WITH (FORCE) (PG13+) terminates any lingering connection so the drop
      # doesn't fail on a slow-to-close api/vite child.
      psql_owner postgres -c "DROP DATABASE IF EXISTS \"${db}\" WITH (FORCE)" >/dev/null 2>&1 || true
      echo "dropped ${db}"
    fi
  fi
  ;;

sweep)
  # The machine-wide clear, now something you ask for by name. This used to be
  # what a bare `make dev` did on the way up, so the routine command was also
  # the destructive one.
  sweep_stacks
  echo "swept every dev stack on this machine"
  if [[ "$drop" == "1" ]]; then
    # db-up first: drop_stray_dev_dbs issues every statement through the compose
    # Postgres, so on a stopped stack it would connect to nothing, fail
    # silently, and leave the databases to reappear the moment infra started.
    make db-up >/dev/null
    drop_stray_dev_dbs
    echo "note: the shared 'margince' database is kept — DROP=1 removes only the per-slug ones" >&2
  fi
  ;;

*)
  echo "usage: dev.sh {up|stop|sweep} [slug] [--drop]" >&2
  exit 2
  ;;
esac
