#!/usr/bin/env bash
# check-config.sh — run --check-config for the api and the worker on one
# configuration, and fail when either refuses it.
#
# Each program parses only the settings it reads, so the api's check passes a
# file whose worker-only setting the worker would refuse at its start. A
# rollout that checked the api alone learned that from a worker that would not
# boot. migrate is not run: it reads only the DSNs, which both programs check.
#
# The files default to the dev stack's: config/margince.yaml under
# config/margince.dev.yaml, with .env.local when it exists. CONFIG, CONFIG_FILE
# and ENV_FILE name others; an empty CONFIG_FILE or ENV_FILE leaves that layer
# out. Without a MARGINCE_DSN a placeholder is passed: the check never connects.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
cd "$root"

config="${CONFIG:-config/margince.yaml}"
config_file="${CONFIG_FILE-config/margince.dev.yaml}"
env_file="${ENV_FILE-.env.local}"

if [[ ! -f "$config" ]]; then
  echo "FAIL: $config does not exist. Run 'make dev' once to create it, or name a file with CONFIG=<path>." >&2
  exit 1
fi
if [[ -n "${ENV_FILE:-}" && ! -f "$env_file" ]]; then
  echo "FAIL: ENV_FILE=$env_file does not exist." >&2
  exit 1
fi
[[ -f "$env_file" ]] || env_file=""
# A layer left out is passed as an empty flag: an absent one would let
# MARGINCE_CONFIG_FILE or MARGINCE_ENV_FILE choose a file instead.
args=(--config "$config" "--config-file=$config_file" "--env-file=$env_file")
# A DSN any layer sets, even an empty one, keeps the placeholder out, so the
# DSN a start would read is the one checked. A loose match can only make the
# check stricter, never let a broken DSN pass.
yaml_files=("$config")
[[ -n "$config_file" && -f "$config_file" ]] && yaml_files+=("$config_file")
if [[ -z "${MARGINCE_DSN+set}" ]] &&
  ! { [[ -n "$env_file" ]] && grep -Eq '^[[:space:]]*(export[[:space:]]+)?MARGINCE_DSN[[:space:]]*=' "$env_file"; } &&
  ! grep -Eq '^dsn[[:space:]]*:' "${yaml_files[@]}"; then
  args+=(--dsn postgres://localhost/check)
fi

bin="$(mktemp -d)"
trap 'rm -rf "$bin"' EXIT
make -s -C backend composition >/dev/null
( cd backend && GOWORK="$root/build/composition/go.work" GOFLAGS="-overlay=$root/build/overlay/overlay.json" go build -o "$bin/" ./cmd/api ./cmd/worker )

failed=()
for program in api worker; do
  echo "=== $program"
  "$bin/$program" "${args[@]}" --check-config || failed+=("$program")
done

if [[ ${#failed[@]} -gt 0 ]]; then
  echo "FAIL: the configuration is not valid for: ${failed[*]}" >&2
  exit 1
fi
echo "OK: the configuration is valid for the api and the worker"
