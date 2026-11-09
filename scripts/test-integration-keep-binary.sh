#!/usr/bin/env bash
# The `go test -exec` program of the integration build (INTEGRATION_BUILD_OUT in
# scripts/test-integration-parallel.sh). go test hands it each linked test binary
# in place of running it, from the package's own directory, and it keeps the
# binary at LANE_BUILD_OUT/<package dir>/integration.test for a shard to run.
#
# Usage (by go test only): test-integration-keep-binary.sh <test binary> [test flags...]
set -euo pipefail

: "${LANE_MODULE_DIR:?the module directory the package paths are relative to}"
: "${LANE_BUILD_OUT:?the directory that receives the binaries}"
binary="${1:?go test passes the test binary first}"

module="$(cd "$LANE_MODULE_DIR" && pwd -P)"
here="$(pwd -P)"
case "$here" in
  "$module") rel="." ;;
  "$module"/*) rel="./${here#"$module"/}" ;;
  *) echo "FAIL: go test ran $binary from $here, outside the module $module" >&2; exit 1 ;;
esac

mkdir -p "$LANE_BUILD_OUT/$rel"
cp "$binary" "$LANE_BUILD_OUT/$rel/integration.test"
