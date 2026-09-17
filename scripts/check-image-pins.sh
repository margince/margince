#!/usr/bin/env bash
# check-image-pins.sh — fail unless every workflow `uses:` action, every
# container `image:` (workflow service containers + the compose dev stack) AND
# every Dockerfile base `FROM` is pinned to an immutable ref (supply-chain: a symbolic ref lets a compromised
# artifact ride into CI unreviewed).
# Allows:  @<40-char-hex>       (git commit SHA, e.g. actions/checkout@11bd71...)
#          @<64-char-hex>       (SHA-256 object id)
#          @sha256:<hex>        (image digest; tag@digest is the readable form)
#          ./path               (local composite action — pinned by this repo)
# Rejects: everything else — tags (@v4, redis:7), branches (@main), and an
#          unpinned ref with no @ at all. An allowlist, not a denylist: a ref
#          shape we didn't anticipate fails closed.
set -euo pipefail

fail=0
workflow_dir=".github/workflows"
actions_dir=".github/actions"
compose_files="docker-compose.dev.yml"

if [[ ! -d "$workflow_dir" ]]; then
  echo "No $workflow_dir directory found — skipping image-pin check"
  exit 0
fi

# Scan the local composite actions alongside the workflows. The `./path` case
# below waves a local action through on the grounds that the repo versions its
# own code — which is only true of the action's OWN ref, not of the third-party
# actions it calls. A composite action pulls those into CI exactly as a workflow
# does, so an unpinned `uses:` one level down would otherwise ride in unread.
scan_dirs=("$workflow_dir")
[[ -d "$actions_dir" ]] && scan_dirs+=("$actions_dir")

# --- Workflow `uses:` actions: pinned to a commit SHA or digest ---
while IFS= read -r line; do
  # Extract the part after `uses:` to check the pin
  pin=$(echo "$line" | sed 's/.*uses:[[:space:]]*//' | cut -d'#' -f1 | tr -d ' "'"'")
  case "$pin" in
    ./*) continue ;; # local composite action: versioned with the repo itself
  esac
  ref="${pin##*@}"
  if [[ "$ref" = "$pin" ]] || ! grep -qE '^([0-9a-f]{40}|[0-9a-f]{64}|sha256:[0-9a-f]{64})$' <<<"$ref"; then
    echo "UNPINNED REF: $line" >&2
    fail=1
  fi
# Match `uses:` only as a YAML key: optional indentation, an optional list
# marker, then the key — never a bare substring. Anchoring this way keeps the
# `statuses:` permission (`stat[uses:]`), a `uses:` inside a comment, and a
# `uses:` in a scalar value from being flagged as an unpinned action.
done < <(grep -rnE --include='*.yml' --include='*.yaml' \
  '^[[:space:]]*(-[[:space:]]+)?uses:' "${scan_dirs[@]}" 2>/dev/null || true)

# --- Container images (workflow services + compose): pinned by digest ---
# A tag pin is not enough for images: tags are mutable, only @sha256: binds
# the bytes. Commented-out lines (e.g. the compose file's parked MinIO block)
# are skipped; everything else fails closed.
while IFS= read -r line; do
  content="${line#*:}"                              # strip the file:lineno prefix
  content="${content#*:}"
  case "$(echo "$content" | sed 's/^[[:space:]]*//')" in
    \#*) continue ;;                                # commented out — not pulled
  esac
  image=$(echo "$content" | sed 's/.*image:[[:space:]]*//' | cut -d'#' -f1 | tr -d ' "'"'")
  if ! grep -qE '@sha256:[0-9a-f]{64}$' <<<"$image"; then
    echo "UNPINNED IMAGE (pin as tag@sha256:<digest>): $line" >&2
    fail=1
  fi
# `image:` as a YAML key too (same key-anchoring as `uses:` above).
done < <(grep -rnE --include='*.yml' --include='*.yaml' \
  '^[[:space:]]*(-[[:space:]]+)?image:' "${scan_dirs[@]}" $compose_files 2>/dev/null || true)

# --- Dockerfile base images: pinned by digest, same rule ---
# A `FROM` is the same supply-chain edge as a service container: the bytes a
# build runs on. It was outside this check while the images were tag-pinned,
# which is exactly the state a gate is for.
#
# Two shapes are waved through, and neither is an upstream pull:
#   * a stage referring to an earlier stage in the same file (`FROM gobase`) —
#     the stage names are collected first so this cannot be confused with an
#     unpinned image that happens to share a name;
#   * `FROM scratch`, which pulls nothing at all.
dockerfiles=$(git ls-files '*Dockerfile' '*Dockerfile.*' 2>/dev/null || true)
for dockerfile in $dockerfiles; do
  stages=$(grep -oiE '[[:space:]]AS[[:space:]]+[A-Za-z0-9_.-]+' "$dockerfile" |
    awk '{print $2}' | tr 'A-Z' 'a-z' || true)
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    lineno="${line%%:*}"
    content="${line#*:}"
    # The image is the first word after FROM, past any --platform= flags.
    image=$(echo "$content" | sed -E 's/^[[:space:]]*[Ff][Rr][Oo][Mm][[:space:]]+//' |
      tr -s ' ' '\n' | grep -v '^--' | head -1)
    [[ "$image" = "scratch" ]] && continue
    if grep -qix "$image" <<<"$stages"; then
      continue                                      # an earlier stage in this file
    fi
    if ! grep -qE '@sha256:[0-9a-f]{64}$' <<<"$image"; then
      echo "UNPINNED BASE IMAGE (pin as tag@sha256:<digest>): $dockerfile:$lineno: $content" >&2
      fail=1
    fi
  done < <(grep -nE '^[[:space:]]*[Ff][Rr][Oo][Mm][[:space:]]' "$dockerfile" || true)
done

if [[ "$fail" -eq 0 ]]; then
  echo "image pins OK"
fi
exit "$fail"
