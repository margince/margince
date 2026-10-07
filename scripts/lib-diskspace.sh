#!/usr/bin/env bash
# The disk check every local lane runs before it starts work. Source this; don't
# execute it.
#
# A full disk does not announce itself. When this machine last filled, Docker
# Desktop stopped, `pg_isready` went quiet, and an integration lane failed with a
# connection error that reads exactly like a broken test fixture; a `git worktree
# add` failed with "could not reset index file". Two sessions independently
# treated it as a defect in their own branch before either measured the disk.
# That is the expensive part, not the clearing.
#
# The cache is the whole story. It reached 267 GB once and 248 GB six days later,
# against 3.7 GB of module cache and 1.5 GB of worktrees. Go trims only when its
# own marker is over 24 hours old, and a machine building all day never lets it,
# so the accumulation is a standing condition rather than an incident.
#
# This warns and refuses; it does not clear anything. Clearing the cache under a
# lane that is already running breaks that lane mid-build with "no such file or
# directory" on a cache entry — measured, by doing it.

# disk_free_bytes PATH — bytes available on the volume holding PATH.
#
# `df -Pk` for POSIX single-line output with 1K blocks: the default -h rounds to
# a unit this cannot compare, and BSD and GNU df disagree on everything except
# -P.
disk_free_bytes() {
  local path="${1:-.}" blocks
  blocks="$(df -Pk "$path" 2>/dev/null | awk 'NR == 2 { print $4 }')"
  [[ -n "$blocks" ]] || return 1
  echo $((blocks * 1024))
}

# go_build_cache_bytes — the Go build cache's size, or empty when it cannot be
# read. Best-effort: it is named in the message as the likeliest cause, and a du
# that fails must not take the lane with it.
go_build_cache_bytes() {
  local cache
  cache="$(go env GOCACHE 2>/dev/null)" || return 1
  [[ -n "$cache" && -d "$cache" ]] || return 1
  du -sk "$cache" 2>/dev/null | awk '{ print $1 * 1024 }'
}

# as_gib BYTES — one decimal place, for a message a human reads.
as_gib() { awk -v b="$1" 'BEGIN { printf "%.1f", b / 1073741824 }'; }

# DISK_FLOOR_GIB refuses below this; DISK_WARN_GIB says so out loud above it.
#
# The floor is what one lane needs to finish: a migrated template clone per
# package plus a cold build. The warning sits two days ahead of it at the
# observed 13 GB/day, which is what makes it a warning rather than a second
# floor — there is time to finish what you are doing.
: "${DISK_FLOOR_GIB:=5}"
: "${DISK_WARN_GIB:=25}"

# require_disk_headroom CONTEXT — refuse to start CONTEXT on a disk about to
# fill, and name the cache rather than leaving the next reader to find it.
#
# CI is exempt: a hosted runner is sized for one job and discarded, so its
# free space is a fact about the runner rather than a condition that creeps up.
# This is about the machine that builds all day.
require_disk_headroom() {
  local context="${1:-this lane}" free floor warn cache
  [[ "${CI:-}" = "true" ]] && return 0
  free="$(disk_free_bytes .)" || return 0
  floor=$((DISK_FLOOR_GIB * 1073741824))
  warn=$((DISK_WARN_GIB * 1073741824))
  (( free >= warn )) && return 0

  cache="$(go_build_cache_bytes || true)"
  local carrying=""
  [[ -n "$cache" ]] && carrying=" ($(as_gib "$cache") GiB of it is the Go build cache)"

  if (( free >= floor )); then
    echo "WARNING: $(as_gib "$free") GiB free${carrying}." >&2
    echo "  Clear it before it bites:  go clean -cache" >&2
    echo "  A full disk arrives as Postgres being unreachable, not as a disk error." >&2
    return 0
  fi
  echo "FAIL: $(as_gib "$free") GiB free — not enough to run ${context}${carrying}." >&2
  echo "  Clear the cache:  go clean -cache" >&2
  echo "  It is pure regenerable state; expect a cold rebuild and an IO-bound few minutes." >&2
  echo "  Refusing here rather than failing later as Postgres being unreachable." >&2
  return 1
}
