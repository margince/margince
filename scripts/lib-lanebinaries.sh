# shellcheck shell=bash
# The integration lane's compiled test binaries: the directory
# INTEGRATION_BUILD_OUT fills, and the check INTEGRATION_BIN_DIR makes before
# a shard runs from it (scripts/test-integration-parallel.sh). Unit-tested by
# scripts/test-lane-binaries.sh.

# The file the build writes first into its directory. It marks the directory
# as the build's own, so a second build may empty it, and it records the
# source the binaries were compiled from.
LANE_BIN_STAMP=.integration-build

# What the binaries are compiled from: the backend module and the composed
# workspace around it.
LANE_SOURCE_PATHS=(backend extensions composition go.work go.work.sum)

# lane_source_stamp ROOT — the commit, plus a digest of every uncommitted
# change and untracked file under LANE_SOURCE_PATHS. Two checkouts print the
# same stamp only when they would compile the same binaries.
lane_source_stamp() { # repo-root
  local root="$1" head digest untracked
  head="$(git -C "$root" rev-parse HEAD)"
  untracked="$(git -C "$root" ls-files --others --exclude-standard -- "${LANE_SOURCE_PATHS[@]}")"
  digest="$({
    git -C "$root" diff HEAD --binary -- "${LANE_SOURCE_PATHS[@]}"
    printf '%s\n' "$untracked"
    [[ -z "$untracked" ]] || git -C "$root" hash-object --stdin-paths <<<"$untracked"
  } | git hash-object --stdin)"
  printf 'commit=%s changes=%s\n' "$head" "$digest"
}

# lane_build_dir_prepare DIR ROOT — empty DIR for a build and stamp it. A
# directory that holds files and no stamp is not the build's to delete.
lane_build_dir_prepare() { # dir repo-root
  local dir="$1" root="$2"
  if [[ -e "$dir" && ! -f "$dir/$LANE_BIN_STAMP" && -n "$(ls -A "$dir" 2>/dev/null)" ]]; then
    echo "FAIL: $dir holds files the integration build did not write (it has no $LANE_BIN_STAMP)."
    echo "  Point INTEGRATION_BUILD_OUT at a new or empty directory; the build empties the one it uses."
    return 1
  fi
  rm -rf "$dir"
  mkdir -p "$dir"
  lane_source_stamp "$root" > "$dir/$LANE_BIN_STAMP"
}

# lane_bin_dir_check DIR ROOT — the binaries in DIR were compiled from the
# source this checkout holds now. Without it, a shard run after an edit tests
# the code from before the edit and reports it green.
lane_bin_dir_check() { # dir repo-root
  local dir="$1" root="$2" built now
  if [[ ! -f "$dir/$LANE_BIN_STAMP" ]]; then
    echo "FAIL: $dir has no $LANE_BIN_STAMP, so INTEGRATION_BUILD_OUT did not fill it."
    echo "  Build the binaries first: INTEGRATION_BUILD_OUT=$dir make test-integration"
    return 1
  fi
  built="$(cat "$dir/$LANE_BIN_STAMP")"
  now="$(lane_source_stamp "$root")"
  if [[ "$built" != "$now" ]]; then
    echo "FAIL: the binaries in $dir were compiled from other source than this checkout."
    echo "  built from: $built"
    echo "  checkout:   $now"
    echo "  Build them again: INTEGRATION_BUILD_OUT=$dir make test-integration"
    return 1
  fi
}

# lane_now — seconds since the epoch with a decimal point, whatever the
# caller's locale. Bash prints EPOCHREALTIME with the locale's separator, and
# awk reads "12,5" as 12.
lane_now() {
  local LC_ALL=C
  printf '%s\n' "${EPOCHREALTIME:-$(date +%s)}"
}
