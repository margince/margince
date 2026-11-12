#!/usr/bin/env bash
# check-image-pins.sh — fail unless every workflow `uses:` action, every job
# `container:` and container `image:` (service containers + the compose dev stack) AND
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
# The dev stack, and the compose folder a release packs for operators.
compose_files="docker-compose.dev.yml scripts/deploy/compose/compose.yaml"

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
  # The tag RELEASE pulls nothing: scripts/release-compose.sh swaps it for the
  # pushed digest and refuses to pack without one.
  [[ "$image" =~ ^ghcr\.io/margince/(api|web|worker):RELEASE$ ]] && continue
  if ! grep -qE '@sha256:[0-9a-f]{64}$' <<<"$image"; then
    echo "UNPINNED IMAGE (pin as tag@sha256:<digest>): $line" >&2
    fail=1
  fi
# `image:` as a YAML key too (same key-anchoring as `uses:` above).
done < <(grep -rnE --include='*.yml' --include='*.yaml' \
  '^[[:space:]]*(-[[:space:]]+)?image:' "${scan_dirs[@]}" $compose_files 2>/dev/null || true)

# --- Job containers written as a scalar: the same digest rule ---
# `container: <image>` is the short form of a mapping with an `image:` key, so
# the scan above never sees it. One expression is admitted: the CI image the
# `ci-image` job resolves, read straight or behind a condition that falls back
# to the empty string (an empty name runs the job on the runner). That job
# writes only a name ending in @sha256:<64 hex>, and refuses anything else, so
# what reaches `container:` is a digest either way.
ci_image_ref='(needs\.ci-image\.outputs\.pinned-image|inputs\.ci-image)'
admits_ci_image() {
  local value="${1// /}"
  [[ "$value" =~ ^\$\{\{${ci_image_ref}\}\}$ ]] && return 0
  # `<condition> && <image> || ''`: a falsy condition yields '', never a tag.
  [[ "$value" =~ ^\$\{\{[^|]+\&\&${ci_image_ref}\|\|\'\'\}\}$ ]]
}
while IFS= read -r line; do
  content="${line#*:}"
  content="${content#*:}"
  case "$(echo "$content" | sed 's/^[[:space:]]*//')" in
    \#*) continue ;;
  esac
  value=$(echo "$content" | sed 's/.*container:[[:space:]]*//' | sed 's/[[:space:]]*#.*//' | sed 's/[[:space:]]*$//')
  [[ -z "$value" ]] && continue                     # mapping form: its image: line is checked above
  admits_ci_image "$value" && continue
  if ! grep -qE '@sha256:[0-9a-f]{64}$' <<<"$(tr -d '"'"'" <<<"$value")"; then
    echo "UNPINNED CONTAINER (pin as tag@sha256:<digest>, or read the ci-image job's output): $line" >&2
    fail=1
  fi
done < <(grep -rnE --include='*.yml' --include='*.yaml' \
  '^[[:space:]]*container:' "${scan_dirs[@]}" 2>/dev/null || true)

# The lane workflows take the CI image as an input, and the input is only as
# pinned as what the caller passes into it.
while IFS= read -r line; do
  value=$(echo "${line#*ci-image:}" | sed 's/[[:space:]]#.*//' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
  [[ -z "$value" ]] && continue                     # the job key or the input's declaration
  if [[ "${value// /}" != '${{needs.ci-image.outputs.pinned-image}}' ]]; then
    echo "CI IMAGE INPUT NOT FROM THE ci-image JOB: $line" >&2
    fail=1
  fi
done < <(grep -rnE --include='*.yml' --include='*.yaml' \
  '^[[:space:]]*ci-image:' "${scan_dirs[@]}" 2>/dev/null || true)

# The job output itself must be the action's, which refuses any name that does
# not end in a digest. A `ci-image` job that wrote its own `pinned-image` would
# hand any tag to every `container:` admitted above.
while IFS= read -r line; do
  file="${line%%:*}"
  value=$(echo "${line#*pinned-image:}" | sed 's/[[:space:]]*#.*//' | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
  [[ -z "$value" ]] && continue                     # the action's own output declaration
  if ! [[ "${value// /}" =~ ^\$\{\{steps\.[A-Za-z0-9_-]+\.outputs\.pinned-image\}\}$ ]] ||
    ! grep -qE '^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*\./\.github/actions/ci-image[[:space:]]*$' "$file"; then
    echo "CI IMAGE OUTPUT NOT FROM THE ci-image ACTION: $line" >&2
    fail=1
  fi
done < <(grep -rnE --include='*.yml' --include='*.yaml' \
  '^[[:space:]]*pinned-image:' "${scan_dirs[@]}" 2>/dev/null || true)

# And only as pinned as its declaration: a `ci-image` input may be declared for
# `workflow_call` alone, where the scan above sees every caller, and with no
# default but the empty one. A `workflow_dispatch` input or a default would
# hand a tag to `container:` past both scans.
while IFS= read -r finding; do
  echo "CI IMAGE INPUT DECLARED OUTSIDE workflow_call OR WITH A DEFAULT: $finding" >&2
  fail=1
done < <(find "${scan_dirs[@]}" -type f \( -name '*.yml' -o -name '*.yaml' \) -print0 2>/dev/null |
  xargs -0 awk '
    FNR == 1 { trigger = ""; inside = 0 }
    /^[[:space:]]*#/ { next }
    match($0, /^  [a-z_]+:/) && !match($0, /^   /) { trigger = $1 }
    inside && match($0, /[^ ]/) <= depth { inside = 0 }
    inside && /^[[:space:]]+default:/ {
      value = $0; sub(/^[^:]*:[[:space:]]*/, "", value); sub(/[[:space:]]*(#.*)?$/, "", value)
      if (value != "\x27\x27" && value != "\"\"") print FILENAME ":" FNR ": " $0
    }
    /^[[:space:]]+ci-image:[[:space:]]*(#.*)?$/ && match($0, /[^ ]/) > 3 {
      inside = 1; depth = match($0, /[^ ]/)
      if (trigger != "workflow_call:") print FILENAME ":" FNR ": declared under " trigger
    }
  ')

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
