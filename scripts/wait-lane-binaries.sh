#!/usr/bin/env bash
# wait-lane-binaries.sh BUDGET — wait until the integration build of this run
# attempt has uploaded its test binaries, and print the artifact's name. Fails,
# and says why, when the build failed or BUDGET seconds pass first.
#
# An integration shard calls it once its stack and template are up, so that
# work runs while the build compiles (scripts/lib-lanebinaries.sh lane_bin_wait).
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib-lanebinaries.sh"

lane_bin_wait "${GITHUB_RUN_ATTEMPT:?}" "${1:?usage: wait-lane-binaries.sh BUDGET-SECONDS}"
