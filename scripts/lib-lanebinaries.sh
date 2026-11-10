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

# The shards start beside the build and wait here for its binaries. The build
# of attempt A uploads them as test-binaries-integration-A, or, when it fails
# or is cancelled, a small test-binaries-integration-A-failed. The attempt is
# in the name because a job that `Re-run failed jobs` does not run again keeps
# its artifacts in the later attempt.
LANE_BIN_ARTIFACT=test-binaries-integration

# lane_bin_wait ATTEMPT BUDGET — print the name of the binaries artifact the
# shards of ATTEMPT run from, once it is there. Returns 1 and says why when that
# build failed, or when BUDGET seconds pass without binaries.
#
# The artifact list costs no REST call. A REST call (GITHUB_TOKEN allows 1,000
# an hour for the whole repository) answers what the list cannot: whether a
# later attempt runs the build at all, asked at once, and after
# LANE_BIN_REST_AFTER seconds whether a build that left no artifact has ended.
# A REST error only delays that answer.
lane_bin_wait() { # attempt budget
  local attempt="$1" budget="$2" spent=0 interval names listed seen="" earlier="" carried=""
  local poll="${LANE_BIN_POLL_SECONDS:-5}" rest_at="${LANE_BIN_REST_AFTER:-360}" rest_calls=0 build
  (( attempt == 1 )) || rest_at=0
  while :; do
    interval="$poll" listed=""
    if names="$(lane_bin_artifacts)"; then
      listed=1
      names="$(grep "^$LANE_BIN_ARTIFACT-" <<<"$names" || true)"
      [[ "$names" == "$seen" ]] || echo "test-binaries: after ${spent}s the run lists: ${names//$'\n'/ }" >&2
      seen="$names"
      if grep -qx "$LANE_BIN_ARTIFACT-$attempt-failed" <<<"$names"; then
        echo "FAIL: the integration build of attempt $attempt failed or was cancelled, so there are no test binaries to run. Its log names the cause." >&2
        return 1
      fi
      if grep -qx "$LANE_BIN_ARTIFACT-$attempt" <<<"$names"; then
        echo "test-binaries: attempt $attempt built them; ${spent}s waited, $rest_calls REST call(s)" >&2
        echo "$LANE_BIN_ARTIFACT-$attempt"
        return 0
      fi
      earlier="$(sed -n "s/^$LANE_BIN_ARTIFACT-\([0-9][0-9]*\)\$/\1/p" <<<"$names" | LC_ALL=C sort -n | tail -n1)"
    else
      interval=$(( poll * 3 ))
    fi
    if (( spent >= rest_at )); then
      rest_at=$(( spent + 180 )) rest_calls=$(( rest_calls + 1 ))
      if build="$(lane_bin_build_job)"; then
        echo "test-binaries: REST call $rest_calls: attempt $attempt lists the build as '$build'" >&2
        case "$build" in
          carried | none) carried=1 ;;
          "completed success") ;;
          completed*)
            echo "FAIL: the integration build of attempt $attempt ended '${build#completed }' and left no test binaries. Its log names the cause." >&2
            return 1
            ;;
        esac
      fi
    fi
    # Decided on a list read in this round, never on an older one.
    if [[ -n "$carried" && -n "$listed" ]]; then
      if [[ -n "$earlier" ]]; then
        echo "test-binaries: attempt $attempt does not run the build again, so it runs the binaries of attempt $earlier; $rest_calls REST call(s)" >&2
        echo "$LANE_BIN_ARTIFACT-$earlier"
        return 0
      fi
      echo "FAIL: attempt $attempt does not run the integration build again, and it lists no test binaries of an earlier attempt" >&2
      return 1
    fi
    if (( spent + interval > budget )); then
      echo "FAIL: no test binaries from the integration build of attempt $attempt after ${spent}s. The build's own time limit is shorter, so it ended without leaving them." >&2
      return 1
    fi
    sleep "$interval"
    spent=$(( spent + interval ))
  done
}

# lane_bin_artifacts — the names of this run's artifacts, of every attempt, one
# per line. Asks the results service the artifact actions use, with
# ACTIONS_RUNTIME_TOKEN and ACTIONS_RESULTS_URL (.github/actions/results-service-env).
lane_bin_artifacts() {
  local payload ids reply code
  # The token's `scp` claim names the run and the job: Actions.Results:RUN:JOB.
  payload="$(cut -d. -f2 <<<"${ACTIONS_RUNTIME_TOKEN:?}" | tr '_-' '/+')"
  while (( ${#payload} % 4 )); do payload+="="; done
  ids="$(base64 -d <<<"$payload" 2>/dev/null | jq -r '.scp // empty' | tr ' ' '\n' \
    | sed -n 's/^Actions\.Results:\([^:]*\):\([^:]*\)$/{"workflow_run_backend_id":"\1","workflow_job_run_backend_id":"\2"}/p' | head -n1)"
  if [[ -z "$ids" ]]; then
    echo "test-binaries: the runtime token names no run for the results service" >&2
    return 1
  fi
  reply="$(mktemp)"
  code=$(curl -sS -o "$reply" -w '%{http_code}' -X POST --max-time 30 \
    -H "Authorization: Bearer $ACTIONS_RUNTIME_TOKEN" -H 'Content-Type: application/json' --data "$ids" \
    "${ACTIONS_RESULTS_URL%/}/twirp/github.actions.results.api.v1.ArtifactService/ListArtifacts" 2>/dev/null) || code=000
  if [[ "$code" != 200 ]]; then
    echo "test-binaries: the artifact list answered $code: $(head -c 200 "$reply")" >&2
    rm -f "$reply"
    return 1
  fi
  jq -r '.artifacts[]?.name' "$reply"
  rm -f "$reply"
}

# lane_bin_build_job — print how this attempt lists the integration build:
# `STATUS CONCLUSION`, `carried` when it is a job of an earlier attempt that
# this one did not run again (it ended before this attempt made it), or `none`.
# One REST call, with GITHUB_TOKEN and `actions: read`.
lane_bin_build_job() {
  local reply code
  reply="$(mktemp)"
  code=$(curl -sS -o "$reply" -w '%{http_code}' --max-time 30 \
    -H "Authorization: Bearer ${GITHUB_TOKEN:?}" -H 'Accept: application/vnd.github+json' \
    "${GITHUB_API_URL:-https://api.github.com}/repos/${GITHUB_REPOSITORY:?}/actions/runs/${GITHUB_RUN_ID:?}/attempts/${GITHUB_RUN_ATTEMPT:?}/jobs?per_page=100" \
    2>/dev/null) || code=000
  if [[ "$code" != 200 ]]; then
    echo "test-binaries: the REST jobs list answered $code: $(head -c 200 "$reply")" >&2
    rm -f "$reply"
    return 1
  fi
  jq -r '[.jobs[]? | select(.name | test("(^|/ )integration build$"))][0]
    | if . == null then "none"
      elif .completed_at != null and .completed_at < .created_at then "carried"
      else "\(.status) \(.conclusion // "-")" end' "$reply"
  rm -f "$reply"
}
