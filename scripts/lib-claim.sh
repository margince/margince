#!/usr/bin/env bash
# One claim on a path that only one caller can win. Source this; don't execute it.
#
# Not `mkdir`: the uutils coreutils that Ubuntu 26.04 ships exit 0 from a mkdir
# that lost the race to create the same directory, so two callers both win.
# Under noclobber bash opens the file with O_EXCL, and the kernel picks one.

# claim PATH — 0 when this call created PATH, 1 when PATH was already there,
# 2 when it cannot be created at all.
claim() {
  (set -C && : > "$1") 2> /dev/null && return 0
  [[ -e "$1" ]] && return 1
  return 2
}
