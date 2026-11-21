#!/usr/bin/env bash
# Is this machine ready to run a dev stack, and is a running stack ready to use?
# Sourced by dev.sh, seed-dev.sh and verify-boot.sh after lib-devstate.sh; don't
# execute it.

# wait_ready url timeout_s [pid] — 0 once the url answers 2xx (a 401/500/503 is
# not ready), 2 as soon as pid has exited, 1 when the time runs out. A process
# that refused its config at boot will never answer, so waiting out the timeout
# for it only delays its reason. The process is asked first: an old server still
# holding the port can answer 2xx for a launch that has already died.
#
# Each request is bounded too: a proxy that accepts the connection and never
# answers would otherwise hold the loop past its timeout, for ever.
wait_ready() {
  local url="$1" timeout="$2" pid="${3:-}" code deadline=$((SECONDS + $2)) limit
  while :; do
    [[ -z "$pid" ]] || kill -0 "$pid" 2>/dev/null || return 2
    limit=$((deadline - SECONDS))
    ((limit <= 10)) || limit=10
    ((limit >= 1)) || limit=1
    code=$(curl -s --max-time "$limit" -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || true)
    [[ "$code" =~ ^2[0-9]{2}$ ]] && return 0
    ((SECONDS < deadline)) || return 1
    sleep 1
  done
}

# dev_require_app BASE [wait_s] — return 0 once BASE/readyz answers, waiting up
# to wait_s for a stack that is still starting. On a miss, say which side failed.
# The app port is Vite, which proxies /readyz and /v1 to the api; a timeout there
# while the api answers on its own port is a frontend fault, and the two have
# different logs to read.
dev_require_app() {
  local base="$1" wait_s="${2:-${DEV_APP_WAIT_SECONDS:-30}}" fe_port api_port
  wait_ready "${base}/readyz" "$wait_s" && return 0
  echo "FAIL: ${base}/readyz did not answer 2xx within ${wait_s}s" >&2
  fe_port="$(dev_recorded_stack FE_PORT || true)"
  api_port="$(dev_recorded_stack API_PORT || true)"
  if [[ -z "$api_port" || "$base" != "http://localhost:${fe_port}" ]]; then
    echo "  Is the stack up? Start it: make dev" >&2
    return 1
  fi
  dev_explain_unready "$fe_port" "$api_port" "${DEV_SLUG:-}"
  return 1
}

# dev_explain_unready FE_PORT API_PORT [SLUG] — after the app port missed, ask the
# api on its own port now and name the log to read: the frontend's if the api
# answers, the api's if it does not.
dev_explain_unready() {
  local fe_port="$1" api_port="$2" slug_flag=""
  [[ -z "${3:-}" ]] || slug_flag=" DEV_SLUG=$3"
  if wait_ready "http://localhost:${api_port}/readyz" 0; then
    echo "  The api answers on its own port :${api_port}, so the fault is the frontend dev server on :${fe_port} or its proxy to the api." >&2
    echo "  Read its log: make dev-logs${slug_flag} ROLE=fe" >&2
  else
    echo "  The api does not answer on its own port :${api_port} either." >&2
    echo "  Read its log: make dev-logs${slug_flag} ROLE=api" >&2
  fi
}

# process_tree PID — PID and every process below it, parents first.
process_tree() {
  local child
  printf '%s\n' "$1"
  for child in $(pgrep -P "$1" 2>/dev/null || true); do
    process_tree "$child"
  done
}

# run_bounded SECONDS OUT CMD… — run CMD with its output in the file OUT, and
# return its status, or 124 if it still runs after SECONDS. macOS ships no
# timeout(1). The output goes to a file because a $(…) pipe waits for every
# process holding it, so a child left behind would hold the caller past the
# limit. On time-out the whole tree gets TERM, then KILL after one second, since
# `docker compose` is a plugin process below the docker CLI.
run_bounded() {
  local ticks=$(($1 * 10)) out="$2" pid tree status=0
  shift 2
  "$@" >"$out" 2>&1 &
  pid=$!
  while kill -0 "$pid" 2>/dev/null; do
    if ((ticks-- <= 0)); then
      tree="$(process_tree "$pid")"
      # The group's stderr also takes bash's "Terminated" notice for the job.
      {
        # shellcheck disable=SC2086
        kill -TERM $tree || true
        for _ in 1 2 3 4 5 6 7 8 9 10; do
          kill -0 "$pid" || break
          sleep 0.1
        done
        # shellcheck disable=SC2086
        kill -KILL $tree || true
        wait "$pid" || true
      } 2>/dev/null
      return 124
    fi
    sleep 0.1
  done
  wait "$pid" || status=$?
  return "$status"
}

# require_docker_compose — refuse a boot that cannot reach Docker, naming what to
# install or start. A missing Compose plugin otherwise surfaces as the tail of a
# `make db-up` error, and a daemon that hangs (a stopped Docker Desktop, a stale
# socket under WSL) holds the boot with no message at all.
: "${DOCKER_CHECK_SECONDS:=15}"
require_docker_compose() {
  local out status=0 guide="docs/how-to/set-up-windows-wsl-development.md" out_file
  # COMPOSE is the Makefile's own override of the compose command.
  local -a compose=(docker compose)
  [[ -z "${COMPOSE:-}" ]] || read -r -a compose <<<"$COMPOSE"
  if ! command -v docker >/dev/null 2>&1; then
    echo "FAIL: docker is not on PATH. make dev runs Postgres, Redis and MinIO in Docker containers." >&2
    echo "  Install Docker Engine with its Compose plugin, or Docker Desktop: https://docs.docker.com/get-docker/" >&2
    echo "  On Windows, run the stack inside WSL: ${guide}" >&2
    return 1
  fi
  out_file="$(mktemp)"
  run_bounded "$DOCKER_CHECK_SECONDS" "$out_file" "${compose[@]}" version || status=$?
  out="$(cat "$out_file")"
  if ((status != 0)); then
    printf '%s\n' "$out" | tail -n 5 | sed 's/^/    /' >&2
    if ((status == 124)); then
      echo "FAIL: '${compose[*]} version' did not answer within ${DOCKER_CHECK_SECONDS}s" >&2
    else
      echo "FAIL: Docker is installed, but its Compose plugin is not ('${compose[*]} version' failed)" >&2
    fi
    rm -f "$out_file"
    echo "  Docker Desktop ships the plugin. On Linux, install the docker-compose-plugin package: https://docs.docker.com/compose/install/linux/" >&2
    echo "  The old standalone docker-compose (v1) is not enough: the Makefile runs 'docker compose'." >&2
    return 1
  fi
  status=0
  run_bounded "$DOCKER_CHECK_SECONDS" "$out_file" docker info --format '{{.ServerVersion}}' || status=$?
  out="$(cat "$out_file")"
  rm -f "$out_file"
  if ((status != 0)); then
    printf '%s\n' "$out" | tail -n 5 | sed 's/^/    /' >&2
    if ((status == 124)); then
      echo "FAIL: the Docker daemon did not answer 'docker info' within ${DOCKER_CHECK_SECONDS}s" >&2
    else
      echo "FAIL: the Docker daemon is not reachable ('docker info' failed)" >&2
    fi
    echo "  Start Docker. If it is running, the CLI may point at another daemon: 'docker context ls' shows the selected one." >&2
    echo "  More checks, none of which delete anything: ${guide}" >&2
    return 1
  fi
}

# warn_wsl_mounted_checkout DIR — a checkout on a Windows drive under WSL
# (/mnt/c/…) reaches every file through the Windows file bridge: Go builds, pnpm
# installs and Vite's file watching all slow down or miss changes. A warning, not
# a refusal, because the stack still runs.
warn_wsl_mounted_checkout() {
  local dir="$1"
  [[ -n "${WSL_DISTRO_NAME:-}" ]] || grep -qi microsoft /proc/version 2>/dev/null || return 0
  case "$dir" in
    /mnt/[a-z]/*)
      echo "WARNING: this checkout is on a Windows drive (${dir}). Builds, installs and file watching are slow there, and Vite can miss edits." >&2
      echo "  Clone into the WSL Linux file system instead, for example ~/src: docs/how-to/set-up-windows-wsl-development.md" >&2
      ;;
  esac
}
