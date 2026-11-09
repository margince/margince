# shellcheck shell=bash
# The integration lane's compiled test binaries: the directory
# INTEGRATION_BUILD_OUT fills, and the check INTEGRATION_BIN_DIR makes before
# a shard runs from it (scripts/test-integration-parallel.sh). Unit-tested by
# scripts/test-lane-binaries.sh.

# The file the build writes first into its directory. It marks the directory
# as the build's own, so a second build may empty it, and it records the
# source the binaries were compiled from.
LANE_BIN_STAMP=.integration-build

# Every file the compiler reads for a package, one path per line: its Go, cgo
# and embedded files, and the go.mod of its module. With `go list -test`, a
# test variant lists its test files among its GoFiles.
# shellcheck disable=SC2016 # a Go template, which go list expands, not the shell
LANE_LIST_FILES='{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}}
{{end}}{{range .CgoFiles}}{{$d}}/{{.}}
{{end}}{{range .EmbedFiles}}{{$d}}/{{.}}
{{end}}{{with .Module}}{{.GoMod}}
{{end}}'

# lane_source_stamp MODULE_DIR TAGS PACKAGE... — a digest of every local file
# `go test -tags=TAGS` compiles for PACKAGE and its dependencies, plus each
# module's go.sum, the workspace files and the toolchain. It reads what the
# compiler reads, so a generated file git ignores counts the same as a tracked
# one. Files under GOROOT and the module cache are pinned by the toolchain and
# go.sum, which the digest already holds.
lane_source_stamp() { # module-dir tags package...
  local dir="$1" tags="$2" goroot modcache gowork listed files=() f
  shift 2
  goroot="$(cd "$dir" && go env GOROOT)"
  modcache="$(cd "$dir" && go env GOMODCACHE)"
  gowork="$(cd "$dir" && go env GOWORK)"
  listed="$(cd "$dir" && go list -deps -test -tags="$tags" -f "$LANE_LIST_FILES" "$@")" || return 1
  while IFS= read -r f; do
    case "$f" in "" | "$goroot"/* | "$modcache"/*) continue ;; esac
    # A test main go generates lives in the build cache, not on disk.
    [[ -f "$f" ]] || continue
    files+=("$f")
    [[ "$f" != */go.mod || ! -f "${f%.mod}.sum" ]] || files+=("${f%.mod}.sum")
  done <<<"$listed"
  if [[ -n "$gowork" && "$gowork" != off ]]; then
    files+=("$gowork")
    [[ ! -f "$gowork.sum" ]] || files+=("$gowork.sum")
  fi
  local sorted digest
  sorted="$(printf '%s\n' "${files[@]}" | LC_ALL=C sort -u)"
  digest="$({
    (cd "$dir" && go env GOVERSION GOOS GOARCH GOFLAGS)
    printf 'tags=%s\n%s\n' "$tags" "$sorted"
    git hash-object --no-filters --stdin-paths <<<"$sorted"
  } | git hash-object --stdin)"
  printf 'files=%s digest=%s\n' "$(grep -c . <<<"$sorted")" "$digest"
}

# lane_build_dir_prepare DIR STAMP — empty DIR for a build and write STAMP
# into it. A directory that holds files and no stamp is not the build's to
# delete.
lane_build_dir_prepare() { # dir stamp
  local dir="$1" stamp="$2"
  if [[ -e "$dir" && ! -f "$dir/$LANE_BIN_STAMP" && -n "$(ls -A "$dir" 2>/dev/null)" ]]; then
    echo "FAIL: $dir holds files the integration build did not write (it has no $LANE_BIN_STAMP)."
    echo "  Point INTEGRATION_BUILD_OUT at a new or empty directory; the build empties the one it uses."
    return 1
  fi
  rm -rf "$dir"
  mkdir -p "$dir"
  printf '%s\n' "$stamp" > "$dir/$LANE_BIN_STAMP"
}

# lane_bin_dir_check DIR STAMP — the binaries in DIR were compiled from the
# source whose stamp is STAMP. Without it, a shard run after an edit tests the
# code from before the edit and reports it green.
lane_bin_dir_check() { # dir stamp
  local dir="$1" now="$2" built
  if [[ ! -f "$dir/$LANE_BIN_STAMP" ]]; then
    echo "FAIL: $dir has no $LANE_BIN_STAMP, so INTEGRATION_BUILD_OUT did not fill it."
    echo "  Build the binaries first: INTEGRATION_BUILD_OUT=$dir make test-integration"
    return 1
  fi
  built="$(cat "$dir/$LANE_BIN_STAMP")"
  if [[ "$built" != "$now" ]]; then
    echo "FAIL: the binaries in $dir were compiled from other source than this checkout."
    echo "  built from: $built"
    echo "  checkout:   $now"
    echo "  Build them again: INTEGRATION_BUILD_OUT=$dir make test-integration"
    return 1
  fi
}

# lane_elapsed T0 T1 — seconds from T0 to T1 with three decimals. Integer
# arithmetic on the two parts, because bash prints EPOCHREALTIME with the
# locale's decimal separator, and a float parser reads "12,5" as 12.
lane_elapsed() { # t0 t1
  local us=$(( $(lane_micros "$2") - $(lane_micros "$1") ))
  printf '%d.%03d\n' $(( us / 1000000 )) $(( us % 1000000 / 1000 ))
}

# lane_micros T — T, in seconds with either decimal separator, as microseconds.
lane_micros() { # t
  local whole="${1%%[.,]*}" frac
  frac="${1#"$whole"}"
  frac="${frac#[.,]}000000"
  echo $(( 10#$whole * 1000000 + 10#${frac:0:6} ))
}
