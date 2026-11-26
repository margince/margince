#!/usr/bin/env bash
# shard-queue-mode.sh PREFIX — print how this run's integration shards share the
# work: `queue` when the cache service takes a claim under PREFIX, `fixed` when
# it does not.
#
# The integration build asks once and packs the answer with the binaries, so
# every shard of the run works the same way. A run whose token may not write
# caches, or a service that does not answer, gets the fixed split: it is slower
# on uneven runners, never less complete.
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib-shardqueue.sh"

QUEUE_BACKEND=cache QUEUE_PREFIX="${1:?usage: shard-queue-mode.sh PREFIX}"
st=0
QUEUE_RETRY_SECONDS="${QUEUE_RETRY_SECONDS:-60}" queue_take mode || st=$?
case "$st" in
  0) echo queue ;;
  3) echo "shard-queue-mode: the cache service refuses writes from this run; the shards split the lane in a fixed way" >&2
     echo fixed ;;
  *) echo "shard-queue-mode: the claim under $QUEUE_PREFIX ended with status $st; the shards split the lane in a fixed way" >&2
     echo fixed ;;
esac
