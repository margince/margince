#!/usr/bin/env bash
# ci-change-base.sh — print the commit a CI run judges its change against.
#
#   pull_request, schedule  the first parent: the base a pull request's merge
#                           commit sits on, or the commit before main's tip
#   merge_group             the merge group's base
#   workflow_dispatch       the first parent on the default branch; on another
#                           branch, its merge base with the default branch, from
#                           the compare API, because a branch tip that merged
#                           main has main's tip as a parent
#
# A base the clone lacks is fetched, that one commit alone. An event with no rule,
# or a base it cannot reach, exits 1; with --fail-open it prints nothing and
# exits 0, for a caller that has a judgement without a base.
#
# Reads GITHUB_EVENT_NAME, GITHUB_EVENT_PATH, GITHUB_REF, GITHUB_SHA,
# GITHUB_REPOSITORY, GITHUB_API_URL and GH_TOKEN.
#
# Usage: scripts/ci-change-base.sh [--fail-open]

set -euo pipefail

fail_open=0
if [[ "${1:-}" == "--fail-open" ]]; then
  fail_open=1
fi

refuse() {
  echo "ci-change-base: $*" >&2
  if [[ "$fail_open" -eq 1 ]]; then
    exit 0
  fi
  exit 1
}

event_field() {
  jq -r "$1 // empty" "${GITHUB_EVENT_PATH:-/dev/null}" 2>/dev/null || true
}

first_parent() {
  git rev-parse --verify --quiet 'HEAD^1' || refuse "HEAD has no parent in this clone; check out at least two commits"
}

event="${GITHUB_EVENT_NAME:-}"
case "$event" in
  pull_request | schedule)
    base="$(first_parent)"
    ;;
  merge_group)
    base="$(event_field .merge_group.base_sha)"
    ;;
  workflow_dispatch)
    default_branch="$(event_field .repository.default_branch)"
    if [[ -n "$default_branch" && "${GITHUB_REF:-}" == "refs/heads/$default_branch" ]]; then
      base="$(first_parent)"
    else
      base="$(curl -fsS --retry 3 \
        -H "Authorization: Bearer ${GH_TOKEN:-}" \
        -H "Accept: application/vnd.github+json" \
        "${GITHUB_API_URL:-}/repos/${GITHUB_REPOSITORY:-}/compare/$default_branch...${GITHUB_SHA:-}" |
        jq -er .merge_base_commit.sha)" ||
        refuse "the compare API gave no merge base of ${GITHUB_SHA:-HEAD} and '$default_branch'"
    fi
    ;;
  *)
    refuse "no base rule for the '$event' event; add one to this script"
    ;;
esac

if [[ -z "${base:-}" ]]; then
  refuse "the '$event' event names no base"
fi
if ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  git fetch --quiet --no-tags --depth=1 origin "$base" || refuse "cannot fetch the base $base"
fi
echo "$base"
