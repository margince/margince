#!/usr/bin/env bash
# The units a composed build carries, read from the composer's own list.
# Sourced by verify-boot.sh; don't execute it.

# composed_unit_transports REPO_DIR — print the channel providers the composed
# units declare, sorted and unique. The set is build/composition/units.json,
# which gen-composition writes, so an excluded development unit is not read and
# a fetched one is. A missing list or a listed manifest that is not there fails;
# an empty list is a core-only build and prints nothing.
composed_unit_transports() {
  local repo="$1" list="$1/build/composition/units.json" manifests manifest
  local -a paths=()
  if [[ ! -f "$list" ]]; then
    echo "$list is missing: run make composition" >&2
    return 1
  fi
  manifests="$(jq -r '.[].manifest' "$list")" || {
    echo "$list is not the composer's unit list" >&2
    return 1
  }
  while IFS= read -r manifest; do
    [[ -n "$manifest" ]] || continue
    if [[ ! -f "$repo/$manifest" ]]; then
      echo "$list lists $manifest, which is not there: run make composition" >&2
      return 1
    fi
    paths+=("$repo/$manifest")
  done <<<"$manifests"
  ((${#paths[@]} > 0)) || return 0
  jq -r '.channels // [] | .[].provider' "${paths[@]}" | sort -u
}
